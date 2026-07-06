package api

import "time"

// Match mirrors the relevant columns of the `matchs` table.
type Match struct {
	ID               int64      `json:"id"`
	SeasonID         *int64     `json:"season_id"`
	TeamA            *int64     `json:"team_a"`
	TeamAName        string     `json:"team_a_name"`
	TeamAFlag        string     `json:"team_a_flag"`
	TeamB            *int64     `json:"team_b"`
	TeamBName        string     `json:"team_b_name"`
	TeamBFlag        string     `json:"team_b_flag"`
	Status           int        `json:"status"`
	StatusText       string     `json:"status_text"`
	IsPaused         bool       `json:"is_paused"`
	ScoreA           int        `json:"score_a"`
	ScoreB           int        `json:"score_b"`
	MaxRound         int        `json:"max_round"`
	Rules            string     `json:"rules"`
	MapSelectionMode string     `json:"map_selection_mode"`
	ConfigPassword   string     `json:"config_password"`
	Enable           bool       `json:"enable"`
	ServerID         *int64     `json:"server_id"`
	IP               string     `json:"ip"`
	CurrentMap       *int64     `json:"current_map"`
	Startdate        *time.Time `json:"startdate"`
	CreatedAt        *time.Time `json:"created_at"`
}

// CreateMatchRequest is the payload for POST /matches. Match creation is a
// pure DB write; the nodeJS bot picks up any enabled match with status=STARTING
// on its own poll, so no bot integration is needed here.
type CreateMatchRequest struct {
	// Optional team ids. When set, team name/flag are copied from the teams table
	// (overriding TeamAName/TeamBName), matching the Symfony panel behaviour.
	TeamAID *int64 `json:"team_a_id"`
	TeamBID *int64 `json:"team_b_id"`

	TeamAName string `json:"team_a_name"`
	TeamBName string `json:"team_b_name"`
	TeamAFlag string `json:"team_a_flag"`
	TeamBFlag string `json:"team_b_flag"`

	SeasonID *int64 `json:"season_id"`

	MaxRound         int    `json:"max_round"`          // default 30
	Rules            string `json:"rules"`              // required, e.g. "esl5on5"
	MapSelectionMode string `json:"map_selection_mode"` // bo2|bo3_modea|bo3_modeb|normal, default normal
	ConfigPassword   string `json:"config_password"`

	ConfigKnifeRound   *bool `json:"config_knife_round"`
	ConfigOt           *bool `json:"config_ot"`
	ConfigFullScore    *bool `json:"config_full_score"`
	ConfigStreamer     *bool `json:"config_streamer"`
	ConfigSwitchAuto   *bool `json:"config_switch_auto"`
	OvertimeMaxRound   *int  `json:"overtime_max_round"`
	OvertimeStartmoney *int  `json:"overtime_startmoney"`

	// Maps to play. Defaults to a single "tba" map when empty.
	Maps []string `json:"maps"`
	// Side (ct|t) for the first map. Random when empty.
	Side string `json:"side"`

	// Optional server assignment.
	ServerID *int64 `json:"server_id"`
	// When true, the match is created enabled with status=STARTING so the bot
	// starts it immediately. A server is auto-picked if ServerID is not given.
	Start bool `json:"start"`
}

// GameServer mirrors the `servers` table.
type GameServer struct {
	ID       int64  `json:"id"`
	IP       string `json:"ip"`
	Rcon     string `json:"rcon"`
	Hostname string `json:"hostname"`
	TvIP     string `json:"tv_ip"`
}

// ServerRequest is the payload for POST/PUT /servers.
type ServerRequest struct {
	IP       string `json:"ip"`
	Rcon     string `json:"rcon"`
	Hostname string `json:"hostname"`
	TvIP     string `json:"tv_ip"`
}

// Team mirrors the `teams` table.
type Team struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Shorthandle string `json:"shorthandle"`
	Flag        string `json:"flag"`
	Link        string `json:"link"`
}

// Season mirrors the `seasons` table.
type Season struct {
	ID     int64      `json:"id"`
	Name   string     `json:"name"`
	Event  string     `json:"event"`
	Start  *time.Time `json:"start"`
	End    *time.Time `json:"end"`
	Link   string     `json:"link"`
	Logo   string     `json:"logo"`
	Active bool       `json:"active"`
}

// Map mirrors the `maps` table (one row per map in a match).
type Map struct {
	ID          int64  `json:"id"`
	MatchID     int64  `json:"match_id"`
	MapName     string `json:"map_name"`
	Score1      int    `json:"score_1"`
	Score2      int    `json:"score_2"`
	CurrentSide string `json:"current_side"` // ct|t
	Status      int    `json:"status"`
	MapsFor     string `json:"maps_for"` // default|team1|team2
	NbOt        int    `json:"nb_ot"`
	// RoundsPlayed is score_1 + score_2 — a convenience for "what round are we on".
	RoundsPlayed int `json:"rounds_played"`
}

// PlayerStat mirrors the scoreboard columns of the `players` table.
type PlayerStat struct {
	ID          int64  `json:"id"`
	MatchID     int64  `json:"match_id"`
	MapID       int64  `json:"map_id"`
	SteamID     string `json:"steamid"`
	Pseudo      string `json:"pseudo"`
	Team        string `json:"team"` // a|b|other
	FirstSide   string `json:"first_side"`
	CurrentSide string `json:"current_side"`
	Kills       int    `json:"kills"`
	Assists     int    `json:"assists"`
	Deaths      int    `json:"deaths"`
	Points      int    `json:"points"`
	Headshots   int    `json:"hs"`
	Defuse      int    `json:"defuse"`
	Bombe       int    `json:"bombe"`
	TeamKills   int    `json:"tk"`
	FirstKill   int    `json:"firstkill"`
	// nb1..nb5 = eBot's multikill/clutch round counters (raw, as stored).
	Nb1 int `json:"nb1"`
	Nb2 int `json:"nb2"`
	Nb3 int `json:"nb3"`
	Nb4 int `json:"nb4"`
	Nb5 int `json:"nb5"`
}

// RoundResult mirrors the `round_summary` table (one row per completed round).
type RoundResult struct {
	ID           int64  `json:"id"`
	MatchID      int64  `json:"match_id"`
	MapID        int64  `json:"map_id"`
	RoundID      int    `json:"round_id"`
	BombPlanted  bool   `json:"bomb_planted"`
	BombDefused  bool   `json:"bomb_defused"`
	BombExploded bool   `json:"bomb_exploded"`
	WinType      string `json:"win_type"` // bombdefused|bombeexploded|normal|saved
	TeamWin      string `json:"team_win"` // a|b
	CtWin        bool   `json:"ct_win"`
	TWin         bool   `json:"t_win"`
	ScoreA       int    `json:"score_a"`
	ScoreB       int    `json:"score_b"`
	BestKiller   *int64 `json:"best_killer"`
	BestKillerNb int    `json:"best_killer_nb"`
}

// Kill mirrors the `player_kill` table (kill feed).
type Kill struct {
	ID         int64  `json:"id"`
	MatchID    int64  `json:"match_id"`
	MapID      int64  `json:"map_id"`
	RoundID    int    `json:"round_id"`
	KillerName string `json:"killer_name"`
	KillerID   *int64 `json:"killer_id"`
	KillerTeam string `json:"killer_team"`
	KilledName string `json:"killed_name"`
	KilledID   *int64 `json:"killed_id"`
	KilledTeam string `json:"killed_team"`
	Weapon     string `json:"weapon"`
	Headshot   bool   `json:"headshot"`
}

// Match status constants, from lib/model/doctrine/Matchs.class.php.
const (
	StatusNotStarted = 0
	StatusStarting   = 1
	StatusEndMatch   = 13
	StatusArchive    = 14
)

func statusText(s int) string {
	switch s {
	case 0:
		return "Not started"
	case 1:
		return "Starting"
	case 2:
		return "Warmup knife"
	case 3:
		return "Knife"
	case 4:
		return "End knife"
	case 5:
		return "Warmup 1st side"
	case 6:
		return "First side"
	case 7:
		return "Warmup 2nd side"
	case 8:
		return "Second side"
	case 9:
		return "Warmup OT 1st side"
	case 10:
		return "OT first side"
	case 11:
		return "Warmup OT 2nd side"
	case 12:
		return "OT second side"
	case 13:
		return "End match"
	case 14:
		return "Archived"
	default:
		return "Unknown"
	}
}
