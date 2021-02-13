package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Beat struct.
// TODO - Battle should be a battle object
type Beat struct {
	ID          int    `gorm:"column:id" json:"id"`
	Artist      User   `json:"artist"`
	URL         string `gorm:"column:beat_url" json:"url"`
	Votes       int    `json:"votes"`
	BattleID int    `gorm:"column:battle_id" json:"battle_id,omitempty"`
	UserLike  int `json:"user_like"`
	UserVote  int `json:"user_vote"`
	Feedback    string `json:"feedback"`
	Battle      Battle `json:"battle"`
	Voted       bool   `json:"voted"`
	Placement       int   `json:"placement"`
	Index       int   `json:"index"`
}

// SubmitBeat returns a page that allows a user to submit or update their entry.
func SubmitBeat(c echo.Context) error {
	// Check if user is authenticated.
	me := GetUser(c, false)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	// GET battle ID.
	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	ads := GetAdvertisements()
	toast := GetToast(c)
	URL := c.Request().URL.RequestURI()

	// TODO - Reduce strain here (not *).
	// Get battle and check if it's valid. The title thing can probably go to be honest.
	battle := GetBattle(battleID)
	if battle.Title == "" {
		SetToast(c, "404")
		return c.Redirect(302, "/404")
	}

	tpl := "SubmitBeat"
	title := "Submit"
	if strings.Contains(URL, "update") {
		tpl = "UpdateBeat"
		title = "Update"
	}

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":   title + "Entry",
			"Analytics":   analyticsKey,
			"Buttons": title,
		},
		"Battle":  battle,
		"Me":      me,
		"Toast":   toast,
		"Ads":     ads,
	}

	return c.Render(http.StatusOK, tpl, m)
}

// InsertBeat is the post request from SubmitBeat that enter's a user's beat into the database.
func InsertBeat(c echo.Context) error {
	// Check if user is authenticated.
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	// POST battle ID.
	battleID, err := strconv.Atoi(policy.Sanitize(c.Param("id")))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}
	redirectURL := "/beat/" + strconv.Itoa(battleID) + "/submit"

	// EFFI - CAN MAYBE MAKE MORE EFFICIENT BY JOINING BEAT TABLE TO SEE IF ENTERED
	// MIGHT ALLOW ENTRIES PAST DEADLINES IF FORCED ON EDGE CASES
	password := ""
	err = dbRead.QueryRow("SELECT password FROM battles WHERE id = ? AND results = '0'", battleID).Scan(&password)
	if err != nil {
		SetToast(c, "notopen")
		return c.Redirect(302, redirectURL)
	}
	if password != c.FormValue("password") {
		SetToast(c, "password")
		return c.Redirect(302, redirectURL)
	}

	track := policy.Sanitize(c.FormValue("track"))

	/*
		trackURL, err := url.Parse(track)
		if err != nil {
			SetToast(c, "sconly")
			return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
		}

		// PERF - MIGHT IMPACT A LOT
			if !contains(whitelist, strings.TrimPrefix(trackURL.Host, "www.")) {
				SetToast(c, "sconly")
				return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
			}
	*/

	stmt := "INSERT INTO beats(url, battle_id, user_id) VALUES(?,?,?)"
	response := "/successadd"

	// IF EXISTS UPDATE
	if RowExists("SELECT battle_id FROM beats WHERE user_id = ? AND battle_id = ?", me.ID, battleID) {
		stmt = "UPDATE beats SET url=? WHERE battle_id=? AND user_id=?"
		response = "/successupdate"
	}

	ins, err := dbWrite.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer ins.Close()

	ins.Exec(track, battleID, me.ID)

	SetToast(c, response)
	return c.Redirect(302, "/battle/"+strconv.Itoa(battleID))
}

// UpdateBeat is the POST request from SubmitBeat when a user is updating their track.
func UpdateBeat(c echo.Context) error {
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	battleID, err := strconv.Atoi(policy.Sanitize(c.Param("id")))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// MIGHT ALLOW ENTRIES PAST DEADLINES IF FORCED ON EDGE CASES
	password := ""
	err = dbRead.QueryRow("SELECT password FROM battles WHERE id = ? AND results = '0'", battleID).Scan(&password)
	if err != nil {
		SetToast(c, "notopen")
		return c.Redirect(302, "/battle/"+strconv.Itoa(battleID))
	}

	track := policy.Sanitize(c.FormValue("track"))

	/*
		redirectURL := "/beat/" + strconv.Itoa(battleID) + "/update"
			trackURL, err := url.Parse(track)
			if err != nil {
				SetToast(c, "sconly")
				return c.Redirect(302, redirectURL)
			}

			// PERF - MIGHT IMPACT A LOT
				if !contains(whitelist, strings.TrimPrefix(trackURL.Host, "www.")) {
					SetToast(c, "sconly")
					return c.Redirect(302, redirectURL)
				}
	*/

	ins, err := dbWrite.Prepare("UPDATE beats SET url=? WHERE battle_id=? AND user_id=?")
	if err != nil {
		SetToast(c, "nobeat")
		return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
	}
	defer ins.Close()
	ins.Exec(track, battleID, me.ID)

	SetToast(c, "successupdate")
	return c.Redirect(302, "/battle/"+strconv.Itoa(battleID))
}

// DeleteBeat ...
func DeleteBeat(c echo.Context) error {
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID)

	stmt := "DELETE FROM beats WHERE user_id = ? AND battle_id = ?"
	ins, err := dbWrite.Prepare(stmt)
	if err != nil {
		SetToast(c, "validationerror")
		return c.Redirect(302, redirectURL)
	}
	defer ins.Close()
	ins.Exec(me.ID, battleID)

	SetToast(c, "successdel")
	return c.Redirect(302, redirectURL)
}
