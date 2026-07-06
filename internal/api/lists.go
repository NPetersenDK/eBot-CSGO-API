package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// listServers handles GET /servers
func (s *Server) listServers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(),
		"SELECT id, ip, rcon, hostname, COALESCE(tv_ip, '') FROM servers ORDER BY hostname ASC")
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []GameServer{}
	for rows.Next() {
		var sv GameServer
		if err := rows.Scan(&sv.ID, &sv.IP, &sv.Rcon, &sv.Hostname, &sv.TvIP); err != nil {
			s.dbError(w, err)
			return
		}
		out = append(out, sv)
	}
	writeJSON(w, http.StatusOK, out)
}

// getServer handles GET /servers/{id}
func (s *Server) getServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sv, err := s.fetchServer(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sv)
}

// createServer handles POST /servers
func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeServer(w, r)
	if !ok {
		return
	}
	now := time.Now()
	res, err := s.db.ExecContext(r.Context(),
		`INSERT INTO servers (ip, rcon, hostname, tv_ip, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		req.IP, req.Rcon, req.Hostname, nullStr(req.TvIP), now, now)
	if err != nil {
		s.dbError(w, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		s.dbError(w, err)
		return
	}
	sv, err := s.fetchServer(r, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sv)
}

// updateServer handles PUT /servers/{id}
func (s *Server) updateServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	req, ok := decodeServer(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(),
		`UPDATE servers SET ip = ?, rcon = ?, hostname = ?, tv_ip = ?, updated_at = ? WHERE id = ?`,
		req.IP, req.Rcon, req.Hostname, nullStr(req.TvIP), time.Now(), id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// RowsAffected is 0 both when missing and when values are unchanged;
		// disambiguate with an existence check.
		if _, ferr := s.fetchServer(r, id); errors.Is(ferr, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "server not found")
			return
		}
	}
	sv, err := s.fetchServer(r, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sv)
}

// deleteServer handles DELETE /servers/{id}. Any match referencing it has its
// server_id set to NULL by the FK (ON DELETE SET NULL).
func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(), "DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fetchServer(r *http.Request, id int64) (GameServer, error) {
	var sv GameServer
	err := s.db.QueryRowContext(r.Context(),
		"SELECT id, ip, rcon, hostname, COALESCE(tv_ip, '') FROM servers WHERE id = ?", id).
		Scan(&sv.ID, &sv.IP, &sv.Rcon, &sv.Hostname, &sv.TvIP)
	return sv, err
}

// decodeServer parses and validates a server payload. Writes a 400 and returns
// ok=false on any problem.
func decodeServer(w http.ResponseWriter, r *http.Request) (ServerRequest, bool) {
	var req ServerRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return req, false
	}
	req.IP = strings.TrimSpace(req.IP)
	req.Rcon = strings.TrimSpace(req.Rcon)
	req.Hostname = strings.TrimSpace(req.Hostname)
	if req.IP == "" || req.Rcon == "" || req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "ip, rcon and hostname are required")
		return req, false
	}
	return req, true
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// listTeams handles GET /teams
func (s *Server) listTeams(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(),
		"SELECT id, name, shorthandle, flag, COALESCE(link, '') FROM teams ORDER BY name ASC")
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []Team{}
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Shorthandle, &t.Flag, &t.Link); err != nil {
			s.dbError(w, err)
			return
		}
		out = append(out, t)
	}
	writeJSON(w, http.StatusOK, out)
}

// listSeasons handles GET /seasons
func (s *Server) listSeasons(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, name, event, start, end, COALESCE(link, ''), COALESCE(logo, ''), COALESCE(active, 0)
		 FROM seasons ORDER BY id DESC`)
	if err != nil {
		s.dbError(w, err)
		return
	}
	defer rows.Close()

	out := []Season{}
	for rows.Next() {
		var se Season
		var start, end sql.NullTime
		if err := rows.Scan(&se.ID, &se.Name, &se.Event, &start, &end, &se.Link, &se.Logo, &se.Active); err != nil {
			s.dbError(w, err)
			return
		}
		se.Start = nullTime(start)
		se.End = nullTime(end)
		out = append(out, se)
	}
	writeJSON(w, http.StatusOK, out)
}
