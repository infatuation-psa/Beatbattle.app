package main

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Beat struct.
type Beat struct {
	ID          int    `gorm:"column:id" json:"id"`
	Artist      string `json:"artist"`
	URL         string `gorm:"column:beat_url" json:"url"`
	Votes       int    `gorm:"column:votes" json:"votes"`
	ChallengeID int    `gorm:"column:challenge_id" json:"challenge_id,omitempty"`
	UserID      string `gorm:"column:user_id" json:"user_id,omitempty"`
	Color       string `json:"color"`
}

// SubmitBeat ...
func SubmitBeat(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(r.URL.Query().Get(":toast"))

	battleID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	// TODO - Change this GetBattle statement or change GetBattle, this doesn't need a * sql statement.
	battle := GetBattle(db, battleID)
	if battle.Title == "" {
		http.Redirect(w, r, "/404", 302)
		return
	}

	var user = GetUser(w, r)

	URL := r.URL.RequestURI()

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

	tmpl.ExecuteTemplate(w, tpl, m)
}

// InsertBeat ...
func InsertBeat(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}

	battleID, err := strconv.Atoi(policy.Sanitize(r.URL.Query().Get(":id")))
	if err != nil {
		http.Redirect(w, r, "/", 302)
		return
	}

	redirectURL := "/beat/" + strconv.Itoa(battleID) + "/submit"

	// EFFI - CAN MAYBE MAKE MORE EFFICIENT BY JOINING BEAT TABLE TO SEE IF ENTERED
	// MIGHT ALLOW ENTRIES PAST DEADLINES IF FORCED ON EDGE CASES
	password := ""
	err = db.QueryRow("SELECT password FROM challenges WHERE id = ? AND status = 'entry'", battleID).Scan(&password)
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, redirectURL+"/notopen", 302)
		return
	}
	if err == sql.ErrNoRows {
		http.Redirect(w, r, redirectURL+"/notopen", 302)
		return
	}
	if password != r.FormValue("password") {
		http.Redirect(w, r, redirectURL+"/password", 302)
		return
	}

	track := policy.Sanitize(r.FormValue("track"))

	trackURL, err := url.Parse(track)
	if err != nil {
		http.Redirect(w, r, "/beat/"+strconv.Itoa(battleID)+"/submit/sconly", 302)
		return
	}

	// PERF - MIGHT IMPACT A LOT
	if !contains(whitelist, strings.TrimPrefix(trackURL.Host, "www.")) {
		http.Redirect(w, r, "/beat/"+strconv.Itoa(battleID)+"/submit/sconly", 302)
		return
	}

	// PERF - MIGHT BE PERFORMANCE DEGRADING
	resp, err := http.Get(track)
	if err != nil {
		http.Redirect(w, r, "/beat/"+strconv.Itoa(battleID)+"/submit/invalid", 302)
		return
	}
	if resp.Status == "404 Not Found" {
		http.Redirect(w, r, "/beat/"+strconv.Itoa(battleID)+"/submit/invalid", 302)
		return
	}

	stmt := "INSERT INTO beats(url, challenge_id, user_id) VALUES(?,?,?)"
	response := "/successadd"

	// IF EXISTS UPDATE
	if RowExists(db, "SELECT challenge_id FROM beats WHERE user_id = ? AND challenge_id = ?", user.ID, battleID) {
		stmt = "UPDATE beats SET url=? WHERE challenge_id=? AND user_id=?"
		response = "/successupdate"
	}

	ins, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer ins.Close()

	ins.Exec(track, battleID, user.ID)
	http.Redirect(w, r, "/battle/"+strconv.Itoa(battleID)+response, 302)
	return
}

// UpdateBeat updates the beat in the DB
func UpdateBeat(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}

	battleID, err := strconv.Atoi(policy.Sanitize(r.URL.Query().Get(":id")))
	if err != nil {
		http.Redirect(w, r, "/", 302)
		return
	}
	// MIGHT ALLOW ENTRIES PAST DEADLINES IF FORCED ON EDGE CASES
	if !RowExists(db, "SELECT password FROM challenges WHERE id = ? AND status = 'entry'", battleID) {
		http.Redirect(w, r, "battle"+strconv.Itoa(battleID)+"/notopen", 302)
		return
	}

	redirectURL := "/beat/" + strconv.Itoa(battleID) + "/update"

	track := policy.Sanitize(r.FormValue("track"))

	trackURL, err := url.Parse(track)
	if err != nil {
		http.Redirect(w, r, redirectURL+"/sconly", 302)
		return
	}

	// PERF - MIGHT IMPACT A LOT
	if !contains(whitelist, strings.TrimPrefix(trackURL.Host, "www.")) {
		http.Redirect(w, r, redirectURL+"/sconly", 302)
		return
	}

	// PERF - MIGHT BE PERFORMANCE DEGRADING
	resp, err := http.Get(track)
	if err != nil {
		http.Redirect(w, r, redirectURL+"/invalid", 302)
		return
	}
	if resp.Status == "404 Not Found" {
		http.Redirect(w, r, redirectURL+"/invalid", 302)
		return
	}

	ins, err := db.Prepare("UPDATE beats SET url=? WHERE challenge_id=? AND user_id=?")
	if err != nil {
		http.Redirect(w, r, "/beat/"+strconv.Itoa(battleID)+"/submit/nobeat", 302)
	}
	defer ins.Close()

	ins.Exec(track, battleID, user.ID)
	http.Redirect(w, r, "/battle/"+strconv.Itoa(battleID)+"/successupdate", 302)
	return
}

// DeleteBeat ...
func DeleteBeat(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}

	battleID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID)

	stmt := "DELETE FROM beats WHERE user_id = ? AND challenge_id = ?"
	ins, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, redirectURL+"/validationerror", 302)
	}
	defer ins.Close()

	ins.Exec(user.ID, battleID)

	http.Redirect(w, r, redirectURL+"/successdel", 302)
	return
}
