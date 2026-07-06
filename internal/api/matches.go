package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var validMapModes = map[string]bool{
	"bo2": true, "bo3_modea": true, "bo3_modeb": true, "normal": true,
}

// listMatches handles GET /matches?status=&limit=&offset=
func (s *Server) listMatches(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(queryInt(r, "limit", 50), 1, 500)
	offset := maxInt(queryInt(r, "offset", 0), 0)

	q := `SELECT id, season_id, team_a, team_a_name, team_a_flag,
	              team_b, team_b_name, team_b_flag, status, is_paused,
	              score_a, score_b, max_round, rules, map_selection_mode,
	              config_password, enable, server_id, ip, current_map,
	              startdate, created_at
	       FROM matchs`
	var args []any
	if sv := r.URL.Query().Get("status"); sv != "" {
		st, err := strconv.Atoi(sv)
		if err != nil {
			writeError(w, http.StatusBadRequest, "status must be an integer")
			return
		}
		q += " WHERE status = ?"
		args = append(args, st)
	}
	q += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []Match{}
	for rows.Next() {
		m, err := scanMatch(rows)
		if err != nil {
			s.dbError(w, err)
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// getMatch handles GET /matches/{id}
func (s *Server) getMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := s.fetchMatch(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// createMatch handles POST /matches — inserts a matchs row plus one maps row
// per requested map, inside a transaction.
func (s *Server) createMatch(w http.ResponseWriter, r *http.Request) {
	var req CreateMatchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	// Defaults + validation.
	if req.MaxRound == 0 {
		req.MaxRound = 30
	}
	if req.Rules == "" {
		writeError(w, http.StatusBadRequest, "rules is required")
		return
	}
	if req.MapSelectionMode == "" {
		req.MapSelectionMode = "normal"
	}
	if !validMapModes[req.MapSelectionMode] {
		writeError(w, http.StatusBadRequest, "map_selection_mode must be one of bo2, bo3_modea, bo3_modeb, normal")
		return
	}
	maps := req.Maps
	if len(maps) == 0 {
		maps = []string{"tba"}
	}
	side := req.Side
	if side != "ct" && side != "t" {
		if randByte()%2 == 0 {
			side = "ct"
		} else {
			side = "t"
		}
	}

	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer tx.Rollback()

	// Copy team name/flag from the teams table when ids are supplied.
	if req.TeamAID != nil {
		if n, f, ok, err := teamNameFlag(ctx, tx, *req.TeamAID); err != nil {
			s.dbError(w, err)
			return
		} else if ok {
			req.TeamAName, req.TeamAFlag = n, f
		}
	}
	if req.TeamBID != nil {
		if n, f, ok, err := teamNameFlag(ctx, tx, *req.TeamBID); err != nil {
			s.dbError(w, err)
			return
		} else if ok {
			req.TeamBName, req.TeamBFlag = n, f
		}
	}

	// Resolve server + status.
	status := StatusNotStarted
	enable := false
	serverID := req.ServerID
	var ip string

	if req.Start {
		status = StatusStarting
		enable = true
		if serverID == nil {
			sid, sip, ok, err := freeServer(ctx, tx)
			if err != nil {
				s.dbError(w, err)
				return
			}
			if !ok {
				writeError(w, http.StatusConflict, "no free server available to start the match")
				return
			}
			serverID, ip = &sid, sip
		}
	}
	if serverID != nil && ip == "" {
		sip, ok, err := serverIP(ctx, tx, *serverID)
		if err != nil {
			s.dbError(w, err)
			return
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "server_id does not exist")
			return
		}
		ip = sip
	}

	now := time.Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO matchs
		(ip, server_id, season_id, team_a, team_a_flag, team_a_name,
		 team_b, team_b_flag, team_b_name, status, is_paused, score_a, score_b,
		 max_round, rules, overtime_startmoney, overtime_max_round,
		 config_full_score, config_ot, config_streamer, config_knife_round,
		 config_switch_auto, config_password, config_authkey, enable,
		 map_selection_mode, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ip, serverID, req.SeasonID, req.TeamAID, req.TeamAFlag, req.TeamAName,
		req.TeamBID, req.TeamBFlag, req.TeamBName, status, false, 0, 0,
		req.MaxRound, req.Rules, req.OvertimeStartmoney, req.OvertimeMaxRound,
		b(req.ConfigFullScore), b(req.ConfigOt), b(req.ConfigStreamer), b(req.ConfigKnifeRound),
		b(req.ConfigSwitchAuto), req.ConfigPassword, genAuthkey(), enable,
		req.MapSelectionMode, now, now)
	if err != nil {
		s.dbError(w, err)
		return
	}
	matchID, err := res.LastInsertId()
	if err != nil {
		s.dbError(w, err)
		return
	}

	// One maps row per requested map; remember the first as current_map.
	var firstMapID int64
	for i, name := range maps {
		mres, err := tx.ExecContext(ctx, `INSERT INTO maps
			(match_id, map_name, score_1, score_2, current_side, status, maps_for, nb_ot, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			matchID, name, 0, 0, side, 0, "default", 0, now, now)
		if err != nil {
			s.dbError(w, err)
			return
		}
		if i == 0 {
			if firstMapID, err = mres.LastInsertId(); err != nil {
				s.dbError(w, err)
				return
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE matchs SET current_map = ? WHERE id = ?`, firstMapID, matchID); err != nil {
		s.dbError(w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		s.dbError(w, err)
		return
	}

	m, err := s.fetchMatch(ctx, matchID)
	if err != nil {
		s.dbError(w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/matches/%d", matchID))
	writeJSON(w, http.StatusCreated, m)
}

// startMatch handles POST /matches/{id}/start — assigns a free server (or the
// one already set) and flips the match to enabled/STARTING for the bot to pick up.
func (s *Server) startMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer tx.Rollback()

	var serverID sql.NullInt64
	err = tx.QueryRowContext(ctx, "SELECT server_id FROM matchs WHERE id = ?", id).Scan(&serverID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		s.dbError(w, err)
		return
	}

	var ip string
	if serverID.Valid {
		sip, ok, err := serverIP(ctx, tx, serverID.Int64)
		if err != nil {
			s.dbError(w, err)
			return
		}
		if ok {
			ip = sip
		}
	} else {
		sid, sip, ok, err := freeServer(ctx, tx)
		if err != nil {
			s.dbError(w, err)
			return
		}
		if !ok {
			writeError(w, http.StatusConflict, "no free server available")
			return
		}
		serverID = sql.NullInt64{Int64: sid, Valid: true}
		ip = sip
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE matchs SET server_id = ?, ip = ?, enable = 1, status = ?, config_authkey = ? WHERE id = ?`,
		serverID.Int64, ip, StatusStarting, genAuthkey(), id); err != nil {
		s.dbError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.dbError(w, err)
		return
	}

	m, err := s.fetchMatch(ctx, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// archiveMatch handles POST /matches/{id}/archive
func (s *Server) archiveMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(),
		"UPDATE matchs SET enable = 0, status = ? WHERE id = ?", StatusArchive, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	m, err := s.fetchMatch(r.Context(), id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// deleteMatch handles DELETE /matches/{id}. Child rows cascade via FK.
func (s *Server) deleteMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(), "DELETE FROM matchs WHERE id = ?", id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func (s *Server) fetchMatch(ctx context.Context, id int64) (Match, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, season_id, team_a, team_a_name, team_a_flag,
	              team_b, team_b_name, team_b_flag, status, is_paused,
	              score_a, score_b, max_round, rules, map_selection_mode,
	              config_password, enable, server_id, ip, current_map,
	              startdate, created_at
	       FROM matchs WHERE id = ?`, id)
	return scanMatch(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMatch(row scanner) (Match, error) {
	var m Match
	var (
		seasonID, teamA, teamB, serverID, currentMap sql.NullInt64
		teamAName, teamAFlag, teamBName, teamBFlag    sql.NullString
		rules, mapMode, configPass, ip                sql.NullString
		isPaused, enable                              sql.NullBool
		startdate, createdAt                          sql.NullTime
	)
	err := row.Scan(&m.ID, &seasonID, &teamA, &teamAName, &teamAFlag,
		&teamB, &teamBName, &teamBFlag, &m.Status, &isPaused,
		&m.ScoreA, &m.ScoreB, &m.MaxRound, &rules, &mapMode,
		&configPass, &enable, &serverID, &ip, &currentMap,
		&startdate, &createdAt)
	if err != nil {
		return Match{}, err
	}
	m.SeasonID = nullI64(seasonID)
	m.TeamA = nullI64(teamA)
	m.TeamB = nullI64(teamB)
	m.ServerID = nullI64(serverID)
	m.CurrentMap = nullI64(currentMap)
	m.TeamAName, m.TeamAFlag = teamAName.String, teamAFlag.String
	m.TeamBName, m.TeamBFlag = teamBName.String, teamBFlag.String
	m.Rules, m.MapSelectionMode = rules.String, mapMode.String
	m.ConfigPassword, m.IP = configPass.String, ip.String
	m.IsPaused, m.Enable = isPaused.Bool, enable.Bool
	m.StatusText = statusText(m.Status)
	m.Startdate = nullTime(startdate)
	m.CreatedAt = nullTime(createdAt)
	return m, nil
}

func teamNameFlag(ctx context.Context, tx *sql.Tx, id int64) (name, flag string, ok bool, err error) {
	err = tx.QueryRowContext(ctx, "SELECT name, flag FROM teams WHERE id = ?", id).Scan(&name, &flag)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return name, flag, true, nil
}

func serverIP(ctx context.Context, tx *sql.Tx, id int64) (ip string, ok bool, err error) {
	err = tx.QueryRowContext(ctx, "SELECT ip FROM servers WHERE id = ?", id).Scan(&ip)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return ip, true, nil
}

// freeServer picks a server not currently used by an in-progress match,
// mirroring executeStart in the Symfony panel.
func freeServer(ctx context.Context, tx *sql.Tx) (id int64, ip string, ok bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id, ip FROM servers
		WHERE id NOT IN (
			SELECT server_id FROM matchs
			WHERE enable = 1 AND status > ? AND status < ? AND server_id IS NOT NULL
		) ORDER BY hostname ASC LIMIT 1`, StatusNotStarted, StatusEndMatch).Scan(&id, &ip)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return id, ip, true, nil
}

func genAuthkey() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%d.%s", time.Now().UnixNano(), hex.EncodeToString(buf))
}

func randByte() byte {
	var b [1]byte
	_, _ = rand.Read(b[:])
	return b[0]
}
