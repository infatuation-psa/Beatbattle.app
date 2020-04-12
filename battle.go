package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator"
)

// Battle ...
type Battle struct {
	Title          string    `gorm:"column:title" json:"title" validate:"required"`
	Rules          string    `gorm:"column:rules" json:"rules" validate:"required"`
	Deadline       time.Time `gorm:"column:deadline" json:"deadline" validate:"required"`
	VotingDeadline time.Time `gorm:"column:voting_deadline" json:"voting_deadline" validate:"required"`
	Attachment     string    `gorm:"column:attachment" json:"attachment"`
	Status         string    `gorm:"column:status" json:"status"`
	Password       string    `gorm:"column:password" json:"password"`
	Host           string    `json:"host"`
	UserID         int       `gorm:"column:user_id" json:"user_id" validate:"required"`
	Entries        int       `json:"entries"`
	ID             int       `gorm:"column:id" json:"id"`
	MaxVotes       int       `gorm:"column:maxvotes" json:"maxvotes" validate:"required"`
}

// ParseDeadline returns a human readable deadline & updates the battle status in the database.
func ParseDeadline(db *sql.DB, deadline time.Time, battleID int, deadlineType string, shortForm bool) string {
	var deadlineParsed string = "Open - "
	var curStatus string

	err := db.QueryRow("SELECT status FROM challenges WHERE id = ?", battleID).Scan(&curStatus)
	if err != nil {
		panic(err.Error())
	}

	if time.Until(deadline) < 0 && curStatus == deadlineType {
		deadlineParsed = "Voting - "
		sql := "UPDATE challenges SET status = 'voting' WHERE id = ?"

		if curStatus == "voting" {
			deadlineParsed = "Battle Finished"
			sql = "UPDATE challenges SET status = 'complete' WHERE id = ?"
		}

		updateStatus, err := db.Prepare(sql)
		if err != nil {
			panic(err.Error())
		}
		defer updateStatus.Close()

		updateStatus.Exec(battleID)

		if curStatus == "voting" {
			return deadlineParsed
		}
	}

	if curStatus == "voting" {
		deadlineParsed = "Voting - "
	}

	now := time.Now()
	diff := deadline.Sub(now)
	days := int(diff.Hours() / 24)
	hours := int(diff.Hours() - float64(days*24))
	minutes := int(diff.Minutes() - float64(days*24*60) - float64(hours*60))

	if days > 0 {
		deadlineParsed += strconv.Itoa(days) + " day"
	}
	if days > 1 {
		deadlineParsed += "s"
	}
	if shortForm && days > 0 {
		return deadlineParsed + " left"
	}

	if hours > 0 {
		if !strings.HasSuffix(deadlineParsed, "- ") {
			deadlineParsed += ", "
		}
		deadlineParsed += strconv.Itoa(hours) + " hour"
	}
	if hours > 1 {
		deadlineParsed += "s"
	}
	if shortForm && hours > 0 {
		return deadlineParsed + " left"
	}

	if minutes > 0 {
		if !strings.HasSuffix(deadlineParsed, "- ") {
			deadlineParsed += ", "
		}
		deadlineParsed += strconv.Itoa(minutes) + " minute"
	}
	if minutes > 1 {
		deadlineParsed += "s"
	}

	return deadlineParsed + " left"
}

// ViewBattles - Retrieves all battles and displays to user. Homepage.
func ViewBattles(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(r.URL.Query().Get(":toast"))

	URL := r.URL.RequestURI()

	tpl := "Index"
	status := "entry"
	title := "Who's The Best Producer?"
	if strings.Contains(URL, "past") {
		tpl = "Past"
		status = "complete"
		title = "Past Battles"
	}

	battles := GetBattles(db, status)

	battlesJSON, err := json.Marshal(battles)
	if err != nil {
		fmt.Println(err)
		return
	}

	var user = GetUser(w, r)

	m := map[string]interface{}{
		"Title":   title,
		"Battles": string(battlesJSON),
		"User":    user,
		"Toast":   toast,
	}

	tmpl.ExecuteTemplate(w, tpl, m)
}

// GetBattles retrieves a battle from the database using an ID.
func GetBattles(db *sql.DB, status string) []Battle {
	query := `
		SELECT challenges.id, challenges.title, challenges.deadline, challenges.voting_deadline, challenges.status, challenges.user_id, users.nickname, COUNT(beats.id) as entry_count
		FROM challenges 
		LEFT JOIN users ON challenges.user_id = users.id 
		LEFT JOIN beats ON challenges.id = beats.challenge_id 
		WHERE challenges.status = ?
        GROUP BY 1
		ORDER BY challenges.deadline`

	if status == "entry" {
		query = `
		SELECT challenges.id, challenges.title, challenges.deadline, challenges.voting_deadline, challenges.status, challenges.user_id, users.nickname, COUNT(beats.id) as entry_count
		FROM challenges 
		LEFT JOIN users ON challenges.user_id = users.id 
		LEFT JOIN beats ON challenges.id = beats.challenge_id 
		WHERE challenges.status = ? OR challenges.status = "voting"
		GROUP BY 1
		ORDER BY challenges.deadline`
	}

	rows, err := db.Query(query, status)

	if err != nil {
		panic(err.Error())
	}
	defer rows.Close()

	battle := Battle{}
	battles := []Battle{}
	for rows.Next() {
		err = rows.Scan(&battle.ID, &battle.Title, &battle.Deadline, &battle.VotingDeadline, &battle.Status, &battle.UserID,
			&battle.Host, &battle.Entries)
		if err != nil {
			panic(err.Error())
		}

		switch battle.Status {
		case "entry":
			battle.Status = ParseDeadline(db, battle.Deadline, battle.ID, "entry", true)
		case "voting":
			battle.Status = ParseDeadline(db, battle.VotingDeadline, battle.ID, "voting", true)
		default:
			battle.Status = "Battle Finished" // Complete case
		}

		battles = append(battles, battle)
	}

	return battles
}

// BattleHTTP - Retreives battle and displays to user.
func BattleHTTP(wr http.ResponseWriter, req *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(req.URL.Query().Get(":toast"))

	battleID, err := strconv.Atoi(req.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(wr, req, "/404", 302)
		return
	}

	// Retrieve battle, return to front page if battle doesn't exist.
	battle := GetBattle(db, battleID)

	if battle.Title == "" {
		http.Redirect(wr, req, "/404", 302)
		return
	}

	// Get beats user has voted for
	var user = GetUser(wr, req)
	var lastVotes []int

	votes, err := db.Query("SELECT beat_id FROM votes WHERE user_id = ? AND challenge_id = ? ORDER BY beat_id", user.ID, battleID)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}
	defer votes.Close()

	for votes.Next() {
		var curBeatID int
		err = votes.Scan(&curBeatID)
		if err != nil {
			panic(err.Error())
		}
		lastVotes = append(lastVotes, curBeatID)
	}

	// Fetch beats in this battle.
	var count int
	order := "ORDER BY RAND()"
	if battle.Status == "Battle Finished" {
		order = "ORDER BY votes DESC"
	}

	query := `
		SELECT beats.id, beats.url, beats.votes, users.nickname, votes.id IS NOT NULL AS voted 
		FROM beats 
		LEFT JOIN users on beats.user_id=users.id
		LEFT JOIN votes on votes.user_id=? AND beats.id=votes.beat_id
		WHERE beats.challenge_id=? ` + order

	rows, err := db.Query(query, user.ID, battleID)
	if err != nil {
		// This doesn't crash anything, but should be avoided.
		fmt.Println(err)
		http.Redirect(wr, req, "/404", 302)
		return
	}
	defer rows.Close()

	submission := Beat{}
	entries := []Beat{}

	for rows.Next() {
		voteID := 0
		err = rows.Scan(&submission.ID, &submission.URL, &submission.Votes, &submission.Artist, &voteID)
		if err != nil {
			panic(err.Error())
		}

		count++

		submission.Color = "black"
		if voteID != 0 {
			submission.Color = "#ff5800"
		}

		if strings.Contains(submission.URL, "/s-") {
			secretURL := strings.Split(submission.URL, "/s-")
			submission.URL = `<iframe width="100%" height="20" scrolling="no" frameborder="no" allow="autoplay" show_user="false" src="https://w.soundcloud.com/player/?url=` + secretURL[0] + `?secret_token=s-` + strings.Trim(secretURL[1], "/") + `&color=%23ff5500&inverse=false&auto_play=false&show_user=false"></iframe>`
		} else {
			submission.URL = `<iframe width="100%" height="20" scrolling="no" frameborder="no" allow="autoplay" src="https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&auto_play=false&show_user=false"></iframe>`
		}

		entries = append(entries, submission)
	}

	battle.Entries = count

	e, err := json.Marshal(entries)
	if err != nil {
		fmt.Println(err)
		return
	}

	hasEntered := RowExists(db, "SELECT challenge_id FROM beats WHERE user_id = ? AND challenge_id = ?", user.ID, battleID)
	isOwner := RowExists(db, "SELECT id FROM challenges WHERE user_id = ? AND id = ?", user.ID, battleID)

	m := map[string]interface{}{
		"Title":         battle.Title,
		"Battle":        battle,
		"Beats":         string(e),
		"User":          user,
		"EnteredBattle": hasEntered,
		"IsOwner":       isOwner,
		"Toast":         toast,
	}

	tmpl.ExecuteTemplate(wr, "Battle", m)
}

// GetBattle retrieves a battle from the database using an ID.
func GetBattle(db *sql.DB, battleID int) Battle {
	battle := Battle{}

	query := `
		SELECT challenges.id, challenges.title, challenges.rules, challenges.deadline, challenges.voting_deadline, challenges.attachment, challenges.status, challenges.password, challenges.maxvotes, challenges.user_id, users.nickname
		FROM challenges 
		LEFT JOIN users ON challenges.user_id = users.id 
        WHERE challenges.id = ?`

	err := db.QueryRow(query, battleID).Scan(&battle.ID,
		&battle.Title, &battle.Rules, &battle.Deadline, &battle.VotingDeadline, &battle.Attachment, &battle.Status,
		&battle.Password, &battle.MaxVotes, &battle.UserID, &battle.Host)

	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	if err == sql.ErrNoRows {
		return battle
	}

	battle.Title = strings.ReplaceAll(battle.Title, "&#39;", "'")
	battle.Rules = strings.ReplaceAll(battle.Rules, "&#39;", "'")
	battle.Host = strings.ReplaceAll(battle.Host, "&#39;", "'")

	switch battle.Status {
	case "entry":
		battle.Status = ParseDeadline(db, battle.Deadline, battleID, "entry", false)
	case "voting":
		battle.Status = ParseDeadline(db, battle.VotingDeadline, battleID, "voting", false)
	default:
		battle.Status = "Battle Finished" // Complete case
	}

	return battle
}

// SubmitBattle ...
func SubmitBattle(w http.ResponseWriter, r *http.Request) {
	toast := GetToast(r.URL.Query().Get(":toast"))
	println(r.URL.Query().Get(":toast"))
	fmt.Print(toast)
	var user = GetUser(w, r)
	m := map[string]interface{}{
		"Title": "Submit Battle",
		"User":  user,
		"Toast": toast,
	}

	tmpl.ExecuteTemplate(w, "SubmitBattle", m)
}

// UpdateBattle ...
func UpdateBattle(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(r.URL.Query().Get(":toast"))

	battleID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	region := r.URL.Query().Get(":region")
	country := r.URL.Query().Get(":country")

	loc, err := time.LoadLocation(policy.Sanitize(region + "/" + country))
	if err != nil {
		loc, _ = time.LoadLocation("America/Toronto")
	}

	battle := GetBattle(db, battleID)
	if battle.Title == "" {
		http.Redirect(w, r, "/404", 302)
		return
	}

	var user = GetUser(w, r)
	if battle.UserID != user.ID {
		http.Redirect(w, r, "/notuser", 302)
		return
	}

	// For time.Parse
	layout := "Jan 2, 2006-03:04 PM"

	deadline := strings.Split(battle.Deadline.In(loc).Format(layout), "-")
	votingDeadline := strings.Split(battle.VotingDeadline.In(loc).Format(layout), "-")

	m := map[string]interface{}{
		"Title":              "Update Battle",
		"Battle":             battle,
		"User":               user,
		"DeadlineDate":       deadline[0],
		"DeadlineTime":       deadline[1],
		"VotingDeadlineDate": votingDeadline[0],
		"VotingDeadlineTime": votingDeadline[1],
		"Toast":              toast,
	}

	tmpl.ExecuteTemplate(w, "UpdateBattle", m)
}

// UpdateBattleDB ...
func UpdateBattleDB(w http.ResponseWriter, r *http.Request) {
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

	curStatus := "entry"
	userID := -1
	err = db.QueryRow("SELECT status, user_id FROM challenges WHERE id = ?", battleID).Scan(&curStatus, &userID)
	if err != nil || userID != user.ID {
		http.Redirect(w, r, "/notuser", 302)
		return
	}

	loc, err := time.LoadLocation(policy.Sanitize(r.FormValue("timezone")))
	if err != nil {
		loc, _ = time.LoadLocation("America/Toronto")
	}

	// For time.Parse
	layout := "Jan 2, 2006 03:04 PM"

	unparsedDeadline := policy.Sanitize(r.FormValue("deadline-date") + " " + r.FormValue("deadline-time"))
	deadline, err := time.ParseInLocation(layout, unparsedDeadline, loc)
	if err != nil || deadline.Before(time.Now()) {
		http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/deadlinebefore", 302)
		return
	}

	unparsedVotingDeadline := policy.Sanitize(r.FormValue("votingdeadline-date") + " " + r.FormValue("votingdeadline-time"))
	votingDeadline, err := time.ParseInLocation(layout, unparsedVotingDeadline, loc)
	if err != nil || votingDeadline.Before(deadline) {
		http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/votedeadlinebefore", 302)
		return
	}

	maxVotes, err := strconv.Atoi(policy.Sanitize(r.FormValue("maxvotes")))
	if err != nil || maxVotes < 1 || maxVotes > 10 {
		http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/maxvotesinvalid", 302)
		return
	}

	attachmentURL, err := url.Parse(policy.Sanitize(r.FormValue("attachment")))
	if err != nil {
		http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/unapprovedurl", 302)
		return
	}

	attachment := ""
	// PERF - MIGHT IMPACT A LOT
	if !contains(whitelist, strings.TrimPrefix(attachmentURL.Host, "www.")) {
		http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/unapprovedurl", 302)
		return
	}

	attachment = policy.Sanitize(r.FormValue("attachment"))

	battle := &Battle{
		Title:          policy.Sanitize(r.FormValue("title")),
		Rules:          policy.Sanitize(r.FormValue("rules")),
		Deadline:       deadline,
		VotingDeadline: votingDeadline,
		Attachment:     attachment,
		Host:           user.Name,
		Password:       policy.Sanitize(r.FormValue("password")),
		MaxVotes:       maxVotes,
		UserID:         user.ID,
	}

	if user.Authenticated && r.Method == "POST" {
		v := validator.New()
		err = v.Struct(battle)

		if err != nil {
			for _, err := range err.(validator.ValidationErrors) {
				fmt.Println(err.Namespace())
			}
			http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/validationerror", 302)
			return
		}

		query := `
				UPDATE challenges 
				SET title = ?, rules = ?, deadline = ?, attachment = ?, password = ?, voting_deadline = ?, maxvotes = ?
				WHERE id = ? and user_id = ?`

		ins, err := db.Prepare(query)
		if err != nil {
			panic(err.Error())
		}
		defer ins.Close()

		ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment, battle.Password, battle.VotingDeadline, battle.MaxVotes, battleID, user.ID)
	} else {
		print("Not post")
		http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/nodata", 302)
		return
	}
	http.Redirect(w, r, "/successupdate", 302)
	return
}

// InsertBattle ...
func InsertBattle(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}

	entries := 0
	err := db.QueryRow("SELECT COUNT(id) FROM challenges WHERE status=? AND user_id=?", "entry", user.ID).Scan(&entries)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	if entries >= 3 {
		http.Redirect(w, r, "/maxbattles", 302)
		return
	}

	loc, err := time.LoadLocation(policy.Sanitize(r.FormValue("timezone")))
	if err != nil {
		loc, _ = time.LoadLocation("America/Toronto")
	}
	// For time.Parse
	layout := "Jan 2, 2006 03:04 PM"

	unparsedDeadline := policy.Sanitize(r.FormValue("deadline-date") + " " + r.FormValue("deadline-time"))
	println(unparsedDeadline)
	deadline, err := time.ParseInLocation(layout, unparsedDeadline, loc)
	if err != nil || deadline.Before(time.Now()) {
		http.Redirect(w, r, "/battle/submit/deadlinebefore", 302)
		return
	}

	unparsedVotingDeadline := policy.Sanitize(r.FormValue("votingdeadline-date") + " " + r.FormValue("votingdeadline-time"))
	votingDeadline, err := time.ParseInLocation(layout, unparsedVotingDeadline, loc)
	if err != nil || votingDeadline.Before(deadline) {
		http.Redirect(w, r, "/battle/submit/votedeadlinebefore", 302)
		return
	}

	maxVotes, err := strconv.Atoi(policy.Sanitize(r.FormValue("maxvotes")))
	if err != nil || maxVotes < 1 || maxVotes > 10 {
		http.Redirect(w, r, "/battle/submit/maxvotesinvalid", 302)
		return
	}

	attachmentURL, err := url.Parse(policy.Sanitize(r.FormValue("attachment")))
	if err != nil {
		http.Redirect(w, r, "/battle/submit/unapprovedurl", 302)
		return
	}

	attachment := ""
	// PERF - MIGHT IMPACT A LOT
	if !contains(whitelist, strings.TrimPrefix(attachmentURL.Host, "www.")) {
		http.Redirect(w, r, "/battle/submit/unapprovedurl", 302)
		return
	}

	attachment = policy.Sanitize(r.FormValue("attachment"))

	battle := &Battle{
		Title:          policy.Sanitize(r.FormValue("title")),
		Rules:          policy.Sanitize(r.FormValue("rules")),
		Deadline:       deadline,
		VotingDeadline: votingDeadline,
		Attachment:     attachment,
		Host:           user.Name,
		Status:         "entry",
		Password:       policy.Sanitize(r.FormValue("password")),
		UserID:         user.ID,
		Entries:        0,
		ID:             0,
		MaxVotes:       maxVotes,
	}

	if user.Authenticated && r.Method == "POST" {
		v := validator.New()
		err = v.Struct(battle)

		if err != nil {
			for _, err := range err.(validator.ValidationErrors) {
				fmt.Println(err.Namespace())
			}
			http.Redirect(w, r, "/battle/submit/validationerror", 302)
			return
		}

		if RowExists(db, "SELECT id FROM challenges WHERE user_id = ? AND title = ?", user.ID, battle.Title) {
			http.Redirect(w, r, "/battle/submit/titleexists", 302)
			return
		}

		stmt := "INSERT INTO challenges(title, rules, deadline, attachment, status, password, user_id, voting_deadline, maxvotes) VALUES(?,?,?,?,?,?,?,?,?)"

		ins, err := db.Prepare(stmt)
		if err != nil {
			panic(err.Error())
		}
		defer ins.Close()

		fmt.Println(battle)

		ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment,
			battle.Status, battle.Password, user.ID, battle.VotingDeadline, battle.MaxVotes)
	} else {
		print("Not post")
		http.Redirect(w, r, "/battle/submit/nodata", 302)
		return
	}
	http.Redirect(w, r, "/successadd", 302)
	return
}

// DeleteBattle ...
func DeleteBattle(w http.ResponseWriter, r *http.Request) {
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

	stmt := "DELETE FROM challenges WHERE user_id = ? AND id = ?"

	ins, err := db.Prepare(stmt)
	if err != nil {
		panic(err.Error())
	}
	defer ins.Close()

	ins.Exec(user.ID, battleID)

	http.Redirect(w, r, "/successdel", 302)
	return
}
