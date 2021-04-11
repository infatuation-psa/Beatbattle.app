package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// Beat struct.
// TODO - Battle should be a battle object
type Beat struct {
	ID        int    `gorm:"column:id" json:"id"`
	Artist    User   `json:"artist"`
	URL       string `gorm:"column:beat_url" json:"url"`
	Votes     int    `json:"votes"`
	BattleID  int    `gorm:"column:battle_id" json:"battle_id,omitempty"`
	UserLike  int    `json:"user_like"`
	UserVote  int    `json:"user_vote"`
	Feedback  string `json:"feedback"`
	Battle    Battle `json:"battle"`
	Voted     bool   `json:"voted"`
	Placement int    `json:"placement"`
	Index     int    `json:"index"`
	Field1    string `gorm:"column:field_1" json:"field_1"`
	Field2    string `gorm:"column:field_2" json:"field_2"`
	Field3    string `gorm:"column:field_3" json:"field_3"`
}

func GetBeat(user User, battle Battle) Beat {
	beat := Beat{}
	query := `SELECT id, url, votes, voted, placement, field_1, field_2, field_3
				FROM beats
				WHERE beats.user_id = ?
				AND beats.battle_id = ?`

	err := dbRead.QueryRow(query, user.ID, battle.ID).
		Scan(&beat.ID, &beat.URL, &beat.Votes,
			&beat.Voted, &beat.Placement, &beat.Field1,
			&beat.Field2, &beat.Field3)
	if err != nil {
		log.Println(err)
	}

	beat.Artist = user
	beat.Battle = battle

	return beat
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

	beat := Beat{}

	tpl := "SubmitBeat"
	title := "Submit"
	if strings.Contains(URL, "update") {
		tpl = "UpdateBeat"
		title = "Update"
		beat = GetBeat(me, battle)
	}

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":     title + "Entry",
			"Analytics": analyticsKey,
			"Buttons":   title,
		},
		"Beat":   beat,
		"Battle": battle,
		"Me":     me,
		"Toast":  toast,
		"Ads":    ads,
	}

	return c.Render(http.StatusOK, tpl, m)
}

// Future function to handle audius/other SC links.
func ProcessBeat(beat *url.URL) string {
	parts := strings.Split(beat.Hostname(), ".")
	domain := parts[len(parts)-2] + "." + parts[len(parts)-1]
	fmt.Println(domain)

	processedURL := beat.String()
	if domain == "goo.gl" {
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				processedURL = req.URL.String()
				return nil
			},
		}

		req, err := http.NewRequest(http.MethodGet, beat.String(), nil)
		if err != nil {
			return ""
		}

		resp, err := client.Do(req)
		if err != nil {
			return ""
		}

		defer resp.Body.Close()
	}

	/* else if domain == "audius.co" {
		headers := map[string][]string{
			"Accept": []string{"text/plain"},
		}

		data := bytes.NewBuffer([]byte{})
		req, err := http.NewRequest("GET", "https://dn-usa.audius.metadata.fyi/v1/resolve?url="+beat.String()+"&app_name=Beatbattle.app", data)
		if err != nil {
			log.Println(err)
		}
		req.Header = headers

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Println(err)
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
		}

		var jsonResp AudiusResolve
		err = json.Unmarshal(body, &jsonResp)
		if err != nil {
			log.Println(err)
		}

		processedURL = `https://audius.co/` + jsonResp.Data.ID
	} */

	return processedURL
}

type AudiusResolve struct {
	Data AudiusResolveData `json:"data"`
}

type AudiusResolveData struct {
	ID string `json:"id"`
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
	trackURL, err := url.Parse(track)
	if err != nil {
		SetToast(c, "sconly")
		return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
	}
	track = ProcessBeat(trackURL)
	field1 := policy.Sanitize(c.FormValue("field_1"))
	field2 := policy.Sanitize(c.FormValue("field_2"))
	field3 := policy.Sanitize(c.FormValue("field_3"))

	/*

		// PERF - MIGHT IMPACT A LOT
			if !contains(whitelist, strings.TrimPrefix(trackURL.Host, "www.")) {
				SetToast(c, "sconly")
				return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
			}
	*/

	stmt := "INSERT INTO beats(url, battle_id, user_id, field_1, field_2, field_3) VALUES(?,?,?,?,?,?)"
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

	ins.Exec(track, battleID, me.ID, field1, field2, field3)

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
	trackURL, err := url.Parse(track)
	if err != nil {
		SetToast(c, "sconly")
		return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
	}
	track = ProcessBeat(trackURL)
	field1 := policy.Sanitize(c.FormValue("field_1"))
	field2 := policy.Sanitize(c.FormValue("field_2"))
	field3 := policy.Sanitize(c.FormValue("field_3"))

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

	ins, err := dbWrite.Prepare("UPDATE beats SET url=?, field_1=?, field_2=?, field_3=? WHERE battle_id=? AND user_id=?")
	if err != nil {
		SetToast(c, "nobeat")
		return c.Redirect(302, "/beat/"+strconv.Itoa(battleID)+"/submit")
	}
	defer ins.Close()
	ins.Exec(track, field1, field2, field3, battleID, me.ID)

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
