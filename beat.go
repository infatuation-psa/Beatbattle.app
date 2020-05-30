package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Beat struct.
type Beat struct {
	ID          int    `gorm:"column:id" json:"id"`
	Artist      string `json:"artist"`
	URL         string `gorm:"column:beat_url" json:"url"`
	Votes       int    `json:"votes"`
	ChallengeID int    `gorm:"column:challenge_id" json:"challenge_id,omitempty"`
	UserID      int    `gorm:"column:user_id" json:"user_id,omitempty"`
	LikeColour  string `json:"like_colour"`
	VoteColour  string `json:"vote_colour"`
	Feedback    string `json:"feedback"`
	Status      string `json:"status"`
	Battle      string `json:"battle"`
}

// SubmitBeat ...
func SubmitBeat(c echo.Context) error {
	user := GetUser(c, false)

	if !user.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	toast := GetToast(c)

	URL := c.Request().URL.RequestURI()

	// TODO - Reduce strain her (not *).
	battle := GetBattle(battleID)
	if battle.Title == "" {
		SetToast(c, "404")
		return c.Redirect(302, "/404")
	}

	if battle.GroupID != 0 {
		hasPermissions := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", user.ID, battle.GroupID)

		if !hasPermissions {
			SetToast(c, "notingroup")
			return c.Redirect(302, "/")
		}
	}

	tpl := "SubmitBeat"
	title := "Submit Beat"
	if strings.Contains(URL, "update") {
		tpl = "UpdateBeat"
		title = "Update Beat"
	}

	m := map[string]interface{}{
		"Title":  title,
		"Battle": battle,
		"User":   user,
		"Toast":  toast,
	}

	return c.Render(http.StatusOK, tpl, m)
}

// InsertBeat ...
func InsertBeat(c echo.Context) error {
	user := GetUser(c, true)

	if !user.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	battleID, err := strconv.Atoi(policy.Sanitize(c.Param("id")))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	battle := GetBattle(battleID)
	if battle.GroupID != 0 {
		hasPermissions := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", user.ID, battle.GroupID)

		if !hasPermissions {
			SetToast(c, "notingroup")
			return c.Redirect(302, "/")
		}
	}

	redirectURL := "/beat/" + strconv.Itoa(battleID) + "/submit"

	// EFFI - CAN MAYBE MAKE MORE EFFICIENT BY JOINING BEAT TABLE TO SEE IF ENTERED
	// MIGHT ALLOW ENTRIES PAST DEADLINES IF FORCED ON EDGE CASES
	password := ""
	err = db.QueryRow("SELECT password FROM challenges WHERE id = ? AND status = 'entry'", battleID).Scan(&password)
	if err != nil {
		SetToast(c, "notopen")
		return c.Redirect(302, redirectURL)
	}
	if password != c.FormValue("password") {
		SetToast(c, "password")
		return c.Redirect(302, redirectURL)
	}

	track := policy.Sanitize(c.FormValue("track"))

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

	// PERF - MIGHT BE PERFORMANCE DEGRADING
	resp, err := http.Get(track)
	if err != nil {
		SetToast(c, "invalid")
		return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
	}
	if resp.Status == "404 Not Found" {
		SetToast(c, "invalid")
		return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
	}

	stmt := "INSERT INTO beats(url, challenge_id, user_id) VALUES(?,?,?)"
	response := "/successadd"

	// IF EXISTS UPDATE
	if RowExists("SELECT challenge_id FROM beats WHERE user_id = ? AND challenge_id = ?", user.ID, battleID) {
		stmt = "UPDATE beats SET url=? WHERE challenge_id=? AND user_id=?"
		response = "/successupdate"
	}

	ins, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer ins.Close()

	ins.Exec(track, battleID, user.ID)

	SetToast(c, response)
	return c.Redirect(302, "/battle/"+strconv.Itoa(battleID))
}

// UpdateBeat updates the beat in the DB
func UpdateBeat(c echo.Context) error {
	user := GetUser(c, true)

	if !user.Authenticated {
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
	err = db.QueryRow("SELECT password FROM challenges WHERE id = ? AND status = 'entry'", battleID).Scan(&password)
	if err != nil {
		SetToast(c, "notopen")
		return c.Redirect(302, "/battle/"+strconv.Itoa(battleID))
	}

	redirectURL := "/beat/" + strconv.Itoa(battleID) + "/update"

	track := policy.Sanitize(c.FormValue("track"))

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

	/* PERF - Check if track URL is valid (doesn't 404)
	resp, err := http.Get(track)
	if err != nil || resp.Status == "404 Not Found" {
		return c.Redirect(302, redirectURL+"/invalid")
		return
	}
	*/

	ins, err := db.Prepare("UPDATE beats SET url=? WHERE challenge_id=? AND user_id=?")
	if err != nil {
		SetToast(c, "nobeat")
		return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
	}
	defer ins.Close()

	ins.Exec(track, battleID, user.ID)
	SetToast(c, "successupdate")
	return c.Redirect(302, "/battle/"+strconv.Itoa(battleID))
}

// DeleteBeat ...
func DeleteBeat(c echo.Context) error {
	user := GetUser(c, true)

	if !user.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID)

	stmt := "DELETE FROM beats WHERE user_id = ? AND challenge_id = ?"
	ins, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "validationerror")
		return c.Redirect(302, redirectURL)
	}
	defer ins.Close()

	ins.Exec(user.ID, battleID)

	SetToast(c, "successdel")
	return c.Redirect(302, redirectURL)
}
