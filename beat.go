package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Beat struct.
type Beat struct {
	ID          int    `gorm:"column:id" json:"id"`
	Artist      User   `json:"artist"`
	URL         string `gorm:"column:beat_url" json:"url"`
	Votes       int    `json:"votes"`
	ChallengeID int    `gorm:"column:challenge_id" json:"challenge_id,omitempty"`
	LikeColour  string `json:"like_colour"`
	VoteColour  string `json:"vote_colour"`
	Feedback    string `json:"feedback"`
	Status      string `json:"status"`
	Battle      string `json:"battle"`
	Voted       bool   `json:"voted"`
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

	if battle.GroupID != 0 {
		hasPermissions := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, battle.GroupID)

		if !hasPermissions {
			SetToast(c, "notingroup")
			return c.Redirect(302, "/")
		}
	}

	tpl := "SubmitBeat"
	title := "Submit"
	if strings.Contains(URL, "update") {
		tpl = "UpdateBeat"
		title = "Update"
	}

	m := map[string]interface{}{
		"Title":   title + "Entry",
		"Battle":  battle,
		"Me":      me,
		"Toast":   toast,
		"Ads":     ads,
		"Buttons": title,
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

	battle := GetBattle(battleID)
	if battle.GroupID != 0 {
		hasPermissions := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, battle.GroupID)

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

	stmt := "INSERT INTO beats(url, challenge_id, user_id) VALUES(?,?,?)"
	response := "/successadd"

	// IF EXISTS UPDATE
	if RowExists("SELECT challenge_id FROM beats WHERE user_id = ? AND challenge_id = ?", me.ID, battleID) {
		stmt = "UPDATE beats SET url=? WHERE challenge_id=? AND user_id=?"
		response = "/successupdate"
	}

	ins, err := db.Prepare(stmt)
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
	err = db.QueryRow("SELECT password FROM challenges WHERE id = ? AND status = 'entry'", battleID).Scan(&password)
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

	ins, err := db.Prepare("UPDATE beats SET url=? WHERE challenge_id=? AND user_id=?")
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

	stmt := "DELETE FROM beats WHERE user_id = ? AND challenge_id = ?"
	ins, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "validationerror")
		return c.Redirect(302, redirectURL)
	}
	defer ins.Close()
	ins.Exec(me.ID, battleID)

	SetToast(c, "successdel")
	return c.Redirect(302, redirectURL)
}
