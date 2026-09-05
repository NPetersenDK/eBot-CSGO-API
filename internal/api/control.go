package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/NPetersenDK/eBot-CSGO-API/internal/ebotcmd"
)

// Live control: the three things the Symfony panel offers on a running match
// that a pure database write cannot do on its own.
//
// Stopping is the only one that needs the bot: it holds the match in memory
// with an RCON connection, so nothing but its own adminStop() releases the
// server and sends mp_restartgame. Reset and restart are plain SQL, mirroring
// executeReset and executeStartRetry in the panel.

// stopReleaseTimeout bounds the wait for the bot to act on a stop command. It
// polls its Redis queue every 50 ms, so this is generous.
const stopReleaseTimeout = 15 * time.Second

// controlEnabled guards the endpoints that need the bot.
func (s *Server) controlEnabled(w http.ResponseWriter) bool {
	if s.cmd == nil {
		writeError(w, http.StatusServiceUnavailable,
			"live control disabled: set REDIS_HOST to reach the bot")
		return false
	}
	return true
}

// matchServer returns the address the bot registered the match under and the
// authkey it decrypts commands with. The address comes from servers.ip, which
// is what keys the bot's authkey map.
func (s *Server) matchServer(ctx context.Context, id int64) (ip, authkey string, enabled bool, status int, err error) {
	var (
		serverIP sql.NullString
		key      sql.NullString
		enable   int
	)
	err = s.db.QueryRowContext(ctx, `SELECT sv.ip, m.config_authkey, m.enable, m.status
		FROM matchs m LEFT JOIN servers sv ON sv.id = m.server_id
		WHERE m.id = ?`, id).Scan(&serverIP, &key, &enable, &status)
	if err != nil {
		return "", "", false, 0, err
	}
	return serverIP.String, key.String, enable == 1, status, nil
}

// sendStop pushes the command and waits for the bot to clear enable, which is
// how it reports that it let the server go.
func (s *Server) sendStop(ctx context.Context, id int64, action string) error {
	ip, authkey, _, status, err := s.matchServer(ctx, id)
	if err != nil {
		return err
	}
	// Below STARTING the bot's pickup query never matched, so no bot holds this
	// match and no command would ever be answered. Clear enable ourselves
	// instead of waiting out the timeout - this is also what rescues a row left
	// enabled-but-not-started by some other writer.
	if status < StatusStarting {
		_, err := s.db.ExecContext(ctx, "UPDATE matchs SET enable = 0 WHERE id = ?", id)
		return err
	}
	if ip == "" {
		return errNoServer
	}
	if err := s.cmd.Send(ctx, id, action, ip, authkey); err != nil {
		return err
	}

	deadline := time.Now().Add(stopReleaseTimeout)
	for {
		var enable int
		if err := s.db.QueryRowContext(ctx, "SELECT enable FROM matchs WHERE id = ?", id).Scan(&enable); err != nil {
			return err
		}
		if enable == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errStopTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

var (
	errNoServer    = errors.New("match has no server assigned")
	errStopTimeout = errors.New("the bot did not release the match; is it running and connected to the same Redis?")
)

// stopMatch handles POST /matches/{id}/stop. ?restart=false sends the panel's
// plain "Stop" instead of "Stop with Restart".
func (s *Server) stopMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok || !s.controlEnabled(w) {
		return
	}
	_, _, live, _, err := s.matchServer(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !live {
		writeError(w, http.StatusConflict, "match is not running")
		return
	}

	action := ebotcmd.ActionStop
	if r.URL.Query().Get("restart") == "false" {
		action = ebotcmd.ActionStopNoRestart
	}
	if err := s.sendStop(r.Context(), id, action); err != nil {
		s.controlError(w, err)
		return
	}
	s.writeMatch(w, r.Context(), id)
}

// resetMatch handles POST /matches/{id}/reset - executeReset in the panel.
func (s *Server) resetMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.resetMatchRows(r.Context(), id); err != nil {
		if errors.Is(err, errNotResettable) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.dbError(w, err)
		return
	}
	s.writeMatch(w, r.Context(), id)
}

var errNotResettable = errors.New("only a stopped, unfinished match can be reset")

func (s *Server) resetMatchRows(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status, enable int
	if err := tx.QueryRowContext(ctx,
		"SELECT status, enable FROM matchs WHERE id = ?", id).Scan(&status, &enable); err != nil {
		return err
	}
	if enable == 1 || status <= StatusNotStarted || status >= StatusEndMatch {
		return errNotResettable
	}

	if _, err := tx.ExecContext(ctx, `UPDATE matchs
		SET score_a = 0, score_b = 0, status = ?, ingame_enable = NULL, is_paused = NULL
		WHERE id = ?`, StatusNotStarted, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE maps SET score_1 = 0, score_2 = 0, nb_ot = 0, status = 0 WHERE match_id = ?", id); err != nil {
		return err
	}
	// Per-round history of the old attempt, keyed through the maps rows.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM maps_score WHERE map_id IN (SELECT id FROM maps WHERE match_id = ?)", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM round_summary WHERE match_id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// restartMatch handles POST /matches/{id}/restart: the whole "play it again"
// sequence in one call - stop with restart, reset, then start on the same
// server. Sequenced here rather than in callers so a half-finished restart
// can't be left behind by a dropped connection.
func (s *Server) restartMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok || !s.controlEnabled(w) {
		return
	}
	_, _, live, _, err := s.matchServer(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		s.dbError(w, err)
		return
	}

	if live {
		if err := s.sendStop(r.Context(), id, ebotcmd.ActionStop); err != nil {
			s.controlError(w, err)
			return
		}
	}
	if err := s.resetMatchRows(r.Context(), id); err != nil && !errors.Is(err, errNotResettable) {
		s.dbError(w, err)
		return
	}
	if err := s.startAfterReset(r.Context(), id); err != nil {
		if errors.Is(err, errServerBusy) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.dbError(w, err)
		return
	}
	s.writeMatch(w, r.Context(), id)
}

var errServerBusy = errors.New("another match is running on that server")

// startAfterReset re-arms a match on the server it already has.
//
// Deliberately not the panel's executeStartRetry: that one only flips enable,
// because it resumes a match that still carries its old status. After a reset
// the status is 0, and the bot's pickup query is `status >= STARTING AND
// enable = 1` - so leaving it at 0 would produce a match the bot never adopts
// while the panel, seeing enable = 1, offers live controls that reach nobody.
func (s *Server) startAfterReset(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var serverID sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT server_id FROM matchs WHERE id = ?", id).Scan(&serverID); err != nil {
		return err
	}
	if !serverID.Valid {
		return errNoServer
	}

	var busy int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM matchs
		WHERE server_id = ? AND id != ? AND enable = 1 AND status < ?`,
		serverID.Int64, id, StatusEndMatch).Scan(&busy); err != nil {
		return err
	}
	if busy > 0 {
		return errServerBusy
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE matchs SET enable = 1, status = ?, config_authkey = ? WHERE id = ?",
		StatusStarting, genAuthkey(), id); err != nil {
		return err
	}
	return tx.Commit()
}

// controlError maps the live-control failures onto status codes.
func (s *Server) controlError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoServer):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, errStopTimeout):
		writeError(w, http.StatusGatewayTimeout, err.Error())
	default:
		s.dbError(w, err)
	}
}

// writeMatch responds with the match's current state.
func (s *Server) writeMatch(w http.ResponseWriter, ctx context.Context, id int64) {
	m, err := s.fetchMatch(ctx, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}
