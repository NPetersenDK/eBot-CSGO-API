package api

import (
	"database/sql"
	"net/http"
	"strconv"
)

// mapIDFilter returns an optional "AND map_id = ?" clause from the ?map_id=
// query param. ok=false means a bad value was supplied (response already written).
func mapIDFilter(w http.ResponseWriter, r *http.Request) (clause string, arg []any, ok bool) {
	v := r.URL.Query().Get("map_id")
	if v == "" {
		return "", nil, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "map_id must be a positive integer")
		return "", nil, false
	}
	return " AND map_id = ?", []any{id}, true
}

// listMatchMaps handles GET /matches/{id}/maps
func (s *Server) listMatchMaps(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, match_id, COALESCE(map_name,''), COALESCE(score_1,0), COALESCE(score_2,0),
		        COALESCE(current_side,''), COALESCE(status,0), COALESCE(maps_for,''), COALESCE(nb_ot,0)
		 FROM maps WHERE match_id = ? ORDER BY id ASC`, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []Map{}
	for rows.Next() {
		var m Map
		if err := rows.Scan(&m.ID, &m.MatchID, &m.MapName, &m.Score1, &m.Score2,
			&m.CurrentSide, &m.Status, &m.MapsFor, &m.NbOt); err != nil {
			s.dbError(w, err)
			return
		}
		m.RoundsPlayed = m.Score1 + m.Score2
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// listMatchPlayers handles GET /matches/{id}/players?map_id=
func (s *Server) listMatchPlayers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	extra, args, ok := mapIDFilter(w, r)
	if !ok {
		return
	}
	q := `SELECT id, match_id, map_id, COALESCE(steamid,''), COALESCE(pseudo,''),
	             COALESCE(team,'other'), COALESCE(first_side,''), COALESCE(current_side,''),
	             COALESCE(nb_kill,0), COALESCE(assist,0), COALESCE(death,0), COALESCE(point,0),
	             COALESCE(hs,0), COALESCE(defuse,0), COALESCE(bombe,0), COALESCE(tk,0),
	             COALESCE(firstkill,0), COALESCE(nb1,0), COALESCE(nb2,0), COALESCE(nb3,0),
	             COALESCE(nb4,0), COALESCE(nb5,0)
	      FROM players WHERE match_id = ?` + extra + ` ORDER BY point DESC, nb_kill DESC`
	rows, err := s.db.QueryContext(r.Context(), q, append([]any{id}, args...)...)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []PlayerStat{}
	for rows.Next() {
		var p PlayerStat
		if err := rows.Scan(&p.ID, &p.MatchID, &p.MapID, &p.SteamID, &p.Pseudo,
			&p.Team, &p.FirstSide, &p.CurrentSide,
			&p.Kills, &p.Assists, &p.Deaths, &p.Points,
			&p.Headshots, &p.Defuse, &p.Bombe, &p.TeamKills,
			&p.FirstKill, &p.Nb1, &p.Nb2, &p.Nb3,
			&p.Nb4, &p.Nb5); err != nil {
			s.dbError(w, err)
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, out)
}

// listMatchRounds handles GET /matches/{id}/rounds?map_id= — round-by-round
// summary, ordered chronologically. This is what tells you the current round.
func (s *Server) listMatchRounds(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	extra, args, ok := mapIDFilter(w, r)
	if !ok {
		return
	}
	q := `SELECT id, match_id, map_id, COALESCE(round_id,0),
	             COALESCE(bomb_planted,0), COALESCE(bomb_defused,0), COALESCE(bomb_exploded,0),
	             COALESCE(win_type,''), COALESCE(team_win,''), COALESCE(ct_win,0), COALESCE(t_win,0),
	             COALESCE(score_a,0), COALESCE(score_b,0), best_killer, COALESCE(best_killer_nb,0)
	      FROM round_summary WHERE match_id = ?` + extra + ` ORDER BY map_id ASC, round_id ASC, id ASC`
	rows, err := s.db.QueryContext(r.Context(), q, append([]any{id}, args...)...)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []RoundResult{}
	for rows.Next() {
		var rr RoundResult
		var bestKiller sql.NullInt64
		if err := rows.Scan(&rr.ID, &rr.MatchID, &rr.MapID, &rr.RoundID,
			&rr.BombPlanted, &rr.BombDefused, &rr.BombExploded,
			&rr.WinType, &rr.TeamWin, &rr.CtWin, &rr.TWin,
			&rr.ScoreA, &rr.ScoreB, &bestKiller, &rr.BestKillerNb); err != nil {
			s.dbError(w, err)
			return
		}
		rr.BestKiller = nullI64(bestKiller)
		out = append(out, rr)
	}
	writeJSON(w, http.StatusOK, out)
}

// listMatchKills handles GET /matches/{id}/kills?map_id= — the kill feed.
func (s *Server) listMatchKills(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	extra, args, ok := mapIDFilter(w, r)
	if !ok {
		return
	}
	q := `SELECT id, match_id, map_id, COALESCE(round_id,0),
	             COALESCE(killer_name,''), killer_id, COALESCE(killer_team,''),
	             COALESCE(killed_name,''), killed_id, COALESCE(killed_team,''),
	             COALESCE(weapon,''), COALESCE(headshot,0)
	      FROM player_kill WHERE match_id = ?` + extra + ` ORDER BY map_id ASC, round_id ASC, id ASC`
	rows, err := s.db.QueryContext(r.Context(), q, append([]any{id}, args...)...)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []Kill{}
	for rows.Next() {
		var k Kill
		var killerID, killedID sql.NullInt64
		if err := rows.Scan(&k.ID, &k.MatchID, &k.MapID, &k.RoundID,
			&k.KillerName, &killerID, &k.KillerTeam,
			&k.KilledName, &killedID, &k.KilledTeam,
			&k.Weapon, &k.Headshot); err != nil {
			s.dbError(w, err)
			return
		}
		k.KillerID = nullI64(killerID)
		k.KilledID = nullI64(killedID)
		out = append(out, k)
	}
	writeJSON(w, http.StatusOK, out)
}
