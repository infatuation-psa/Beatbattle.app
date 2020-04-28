package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator"
	"github.com/gomarkdown/markdown"
)

// Battle ...
type Battle struct {
	Title          string        `gorm:"column:title" json:"title" validate:"required"`
	Rules          string        `gorm:"column:rules" validate:"required"`
	RulesHTML      template.HTML `json:"rules"`
	Deadline       time.Time     `gorm:"column:deadline" json:"deadline" validate:"required"`
	VotingDeadline time.Time     `gorm:"column:voting_deadline" json:"voting_deadline" validate:"required"`
	Attachment     string        `gorm:"column:attachment" json:"attachment"`
	Status         string        `gorm:"column:status"`
	StatusDisplay  string        `json:"status"`
	Password       string        `gorm:"column:password" json:"password"`
	Host           string        `json:"host"`
	UserID         int           `gorm:"column:user_id" json:"user_id" validate:"required"`
	Entries        int           `json:"entries"`
	ID             int           `gorm:"column:id" json:"id"`
	MaxVotes       int           `gorm:"column:maxvotes" json:"maxvotes" validate:"required"`
	Tags           []Tag
}

// Tag ...
type Tag struct {
	Value string `json:"tag"`
}

// ParseDeadline returns a human readable deadline & updates the battle status in the database.
func ParseDeadline(db *sql.DB, deadline time.Time, battleID int, deadlineType string, shortForm bool) string {
	var deadlineParsed string = "Open - "
	var curStatus string

	err := db.QueryRow("SELECT status FROM challenges WHERE id = ?", battleID).Scan(&curStatus)
	if err != nil {
		return ""
	}

	if time.Until(deadline) < 0 && curStatus == deadlineType {
		deadlineParsed = "Voting - "
		sql := "UPDATE challenges SET status = 'voting' WHERE id = ?"

		if curStatus == "voting" {
			deadlineParsed = "Finished -"
			sql = "UPDATE challenges SET status = 'complete' WHERE id = ?"
		}

		updateStatus, err := db.Prepare(sql)
		if err != nil {
			return ""
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

	// Fix bug where none are shown
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

	battles := GetBattles(db, "challenges.status", status)

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

// ViewTaggedBattles - Retrieves all tagged battles and displays to user.
func ViewTaggedBattles(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(r.URL.Query().Get(":toast"))

	title := "Battles Tagged With " + policy.Sanitize(r.URL.Query().Get(":tag"))

	battles := GetBattles(db, "tags.tag", policy.Sanitize(r.URL.Query().Get(":tag")))

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
		"Tag":     policy.Sanitize(r.URL.Query().Get(":tag")),
	}

	tmpl.ExecuteTemplate(w, "ViewBattles", m)
}

// MyAccount - Retrieves all of user's battles and displays to user.
func MyAccount(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(r.URL.Query().Get(":toast"))

	var user = GetUser(w, r)

	battles := GetBattles(db, "challenges.user_id", strconv.Itoa(user.ID))

	battlesJSON, err := json.Marshal(battles)
	if err != nil {
		fmt.Println(err)
		return
	}

	m := map[string]interface{}{
		"Title":   "My Account",
		"Battles": string(battlesJSON),
		"User":    user,
		"Toast":   toast,
		"Tag":     policy.Sanitize(r.URL.Query().Get(":tag")),
	}

	tmpl.ExecuteTemplate(w, "MyAccount", m)
}

// GetBattles retrieves a battle from the database using an ID.
func GetBattles(db *sql.DB, field string, value string) []Battle {
	// FIELD & VALUE
	querySELECT := `SELECT challenges.id, challenges.title, challenges.deadline, challenges.voting_deadline, challenges.status, challenges.user_id, users.nickname, COUNT(beats.id) as entry_count
					FROM challenges 
					LEFT JOIN users ON challenges.user_id = users.id 
					LEFT JOIN beats ON challenges.id = beats.challenge_id`
	queryWHERE := "WHERE " + field + "=?"
	queryGROUP := "GROUP BY 1"
	queryORDER := "ORDER BY challenges.deadline DESC"

	if field == "tags.tag" {
		querySELECT = `SELECT challenges.id, challenges.title, challenges.deadline, challenges.voting_deadline, challenges.status, challenges.user_id, users.nickname, COUNT(beats.id) as entry_count
						FROM tags
						LEFT JOIN challenges_tags ON challenges_tags.tag_id = tags.id
						LEFT JOIN challenges on challenges.id = challenges_tags.challenge_id
						LEFT JOIN users ON challenges.user_id = users.id 
						LEFT JOIN beats ON challenges.id = beats.challenge_id `
		queryORDER = "ORDER BY challenges.deadline ASC"
	}

	if field == "challenges.status" {
		if value == "entry" {
			queryWHERE = queryWHERE + " OR challenges.status = 'voting'"
			queryORDER = "ORDER BY challenges.deadline ASC"
		}

		if value == "past" {
			queryORDER = "ORDER BY challenges.voting_deadline DESC"
		}
	}

	query := querySELECT + " " + queryWHERE + " " + queryGROUP + " " + queryORDER

	rows, err := db.Query(query, value)

	if err != nil {
		return nil
	}
	defer rows.Close()

	battle := Battle{}
	battles := []Battle{}
	for rows.Next() {
		err = rows.Scan(&battle.ID, &battle.Title, &battle.Deadline, &battle.VotingDeadline, &battle.Status, &battle.UserID,
			&battle.Host, &battle.Entries)
		if err != nil {
			return nil
		}

		switch battle.Status {
		case "entry":
			battle.StatusDisplay = ParseDeadline(db, battle.Deadline, battle.ID, "entry", true)
		case "voting":
			battle.StatusDisplay = ParseDeadline(db, battle.VotingDeadline, battle.ID, "voting", true)
		case "draft":
			battle.StatusDisplay = "Draft"
		default:
			layoutUS := "January 2, 2006"
			battle.StatusDisplay = "Finished - " + battle.VotingDeadline.Format(layoutUS) // Complete case
		}

		battle.Tags = GetTags(db, battle.ID)

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
		http.Redirect(wr, req, "/502", 302)
		return
	}
	defer votes.Close()

	for votes.Next() {
		var curBeatID int
		err = votes.Scan(&curBeatID)
		if err != nil {
			http.Redirect(wr, req, "/502", 302)
			return
		}
		lastVotes = append(lastVotes, curBeatID)
	}

	// Fetch beats in this battle.
	var count int

	//Should these reset in rows.next? sbumission at least
	submission := Beat{}
	entries := []Beat{}
	didntVote := []Beat{}
	voteID := 0

	args := []interface{}{user.ID, user.ID, battleID}
	scanArgs := []interface{}{&submission.ID, &submission.URL, &submission.Votes, &submission.Artist, &voteID, &submission.Feedback, &submission.UserID}
	query := `SELECT beats.id, beats.url, beats.votes, users.nickname, votes.id IS NOT NULL AS voted, IFNULL(feedback.feedback, ''), beats.user_id
			FROM beats 
			LEFT JOIN users on beats.user_id=users.id
			LEFT JOIN votes on votes.user_id=? AND beats.id=votes.beat_id
			LEFT JOIN feedback on feedback.user_id=? AND feedback.beat_id=beats.id
			WHERE beats.challenge_id=?
			GROUP BY 1
			ORDER BY beats.id DESC`

	if battle.Status == "complete" {
		query = `SELECT beats.id, beats.url, beats.votes, users.nickname, votes.id IS NOT NULL AS voted, beats.user_id
				FROM beats 
				LEFT JOIN users on beats.user_id=users.id
				LEFT JOIN votes on votes.user_id=beats.user_id AND votes.challenge_id=beats.challenge_id
				WHERE beats.challenge_id=?
				GROUP BY 1
				ORDER BY votes DESC`
		args = []interface{}{battleID}
		scanArgs = []interface{}{&submission.ID, &submission.URL, &submission.Votes, &submission.Artist, &voteID, &submission.UserID}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		// This doesn't crash anything, but should be avoided.
		fmt.Println(err)
		http.Redirect(wr, req, "/404", 302)
		return
	}
	defer rows.Close()

	// PERF
	entryPosition := 0
	hasEntered := false
	userVotes := 0
	for rows.Next() {
		voteID = 0
		err = rows.Scan(scanArgs...)
		if err != nil {
			http.Redirect(wr, req, "/502", 302)
			return
		}

		if battle.Status == "complete" && voteID == 0 {
			submission.Artist = `<span class="tooltipped" data-tooltip="Did Not Vote">` + submission.Artist + ` <span style="color: #1E19FF;">(*)</span></span>`
		}

		count++

		submission.Color = "black"
		if voteID != 0 {
			submission.Color = "#ff5800"
			if battle.Status == "complete" && submission.UserID == user.ID {
				userVotes++
			}
			if battle.Status != "complete" {
				userVotes++
			}
		}

		u, _ := url.Parse(submission.URL)
		urlSplit := strings.Split(u.RequestURI(), "/")

		width := "width='100%'"

		if battle.Status != "complete" {
			width = "width='20px'"
		}

		if len(urlSplit) >= 4 {
			secretURL := urlSplit[3]
			if strings.Contains(secretURL, "s-") {
				submission.URL = `<iframe ` + width + ` height="20" scrolling="no" frameborder="no" allow="autoplay" show_user="false" src="https://w.soundcloud.com/player/?url=https://soundcloud.com/` + urlSplit[1] + "/" + urlSplit[2] + `?secret_token=` + urlSplit[3] + `&color=%23ff5500&inverse=false&auto_play=false&show_user=false"></iframe>`
			} else {
				submission.URL = `<iframe ` + width + ` height="20" scrolling="no" frameborder="no" allow="autoplay" src="https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&auto_play=false&show_user=false"></iframe>`
			}
		} else {
			submission.URL = `<iframe ` + width + ` height="20" scrolling="no" frameborder="no" allow="autoplay" src="https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&auto_play=false&show_user=false"></iframe>`
		}

		if battle.Status == "complete" && voteID == 0 {
			didntVote = append(didntVote, submission)
			if submission.UserID == user.ID {
				hasEntered = true
				entryPosition = len(didntVote)
			}
			continue
		}

		entries = append(entries, submission)

		if submission.UserID == user.ID {
			hasEntered = true
			entryPosition = len(entries)
		}
	}

	if hasEntered && battle.Status == "voting" {
		query := `SELECT count(*)+1
					FROM beats
					WHERE challenge_id=? AND votes > (SELECT votes FROM beats WHERE user_id=? AND challenge_id=?)`

		db.QueryRow(query, battleID, user.ID, battleID).Scan(&entryPosition)
	}

	if hasEntered && battle.Status == "complete" && userVotes == 0 {
		entryPosition += len(entries)
	}

	entries = append(entries, didntVote...)

	battle.Entries = count

	// PERF - Shuffle entries per user.
	if battle.Status != "complete" {
		rand.Seed(int64(user.ID + battle.ID))
		rand.Shuffle(len(entries), func(i, j int) {
			entries[i], entries[j] = entries[j], entries[i]
		})
	}

	e, err := json.Marshal(entries)
	if err != nil {
		http.Redirect(wr, req, "/502", 302)
		return
	}

	isOwner := RowExists(db, "SELECT id FROM challenges WHERE user_id = ? AND id = ?", user.ID, battleID)

	m := map[string]interface{}{
		"Title":          battle.Title,
		"Battle":         battle,
		"Beats":          string(e),
		"User":           user,
		"EnteredBattle":  hasEntered,
		"EntryPosition":  entryPosition,
		"IsOwner":        isOwner,
		"Toast":          toast,
		"VotesRemaining": battle.MaxVotes - userVotes,
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

	if err != nil {
		return battle
	}

	battle.Title = html.UnescapeString(battle.Title)
	battle.Host = html.UnescapeString(battle.Host)

	md := []byte(html.UnescapeString(battle.Rules))
	battle.Rules = html.UnescapeString(battle.Rules)
	battle.RulesHTML = template.HTML(markdown.ToHTML(md, nil, nil))

	switch battle.Status {
	case "entry":
		battle.StatusDisplay = ParseDeadline(db, battle.Deadline, battleID, "entry", false)
	case "voting":
		battle.StatusDisplay = ParseDeadline(db, battle.VotingDeadline, battleID, "voting", false)
	case "draft":
		battle.StatusDisplay = "Draft"
	default:
		layoutUS := "January 2, 2006"
		battle.StatusDisplay = "Finished - " + battle.VotingDeadline.Format(layoutUS) // Complete case
	}

	battle.Tags = GetTags(db, battleID)

	return battle
}

// SubmitBattle ...
func SubmitBattle(w http.ResponseWriter, r *http.Request) {
	toast := GetToast(r.URL.Query().Get(":toast"))

	var user = GetUser(w, r)
	m := map[string]interface{}{
		"Title": "Submit Battle",
		"User":  user,
		"Toast": toast,
	}

	tmpl.ExecuteTemplate(w, "SubmitBattle", m)
}

// TemplateBattle ...
func TemplateBattle(w http.ResponseWriter, r *http.Request) {
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

	tmpl.ExecuteTemplate(w, "TemplateBattle", m)
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
	if attachmentURL.String() != "" {
		if !contains(whitelist, strings.TrimPrefix(attachmentURL.Host, "www.")) {
			http.Redirect(w, r, "/battle/"+r.URL.Query().Get(":id")+"/update/unapprovedurl", 302)
			return
		}
	}

	attachment = policy.Sanitize(r.FormValue("attachment"))

	status := curStatus
	if status == "draft" && r.FormValue("submit") == "PUBLISH" {
		status = "entry"
	}
	if r.FormValue("submit") == "DRAFT" {
		status = "draft"
	}

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
		Status:         status,
	}

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
			SET title = ?, rules = ?, deadline = ?, attachment = ?, password = ?, voting_deadline = ?, maxvotes = ?, status = ?
			WHERE id = ? AND user_id = ?`

	ins, err := db.Prepare(query)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer ins.Close()

	ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment, battle.Password, battle.VotingDeadline, battle.MaxVotes, battle.Status, battleID, user.ID)

	if err != nil {
		http.Redirect(w, r, "/failadd", 302)
		return
	}

	TagsDB(db, true, r.FormValue("tags"), int64(battleID))

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
		http.Redirect(w, r, "/502", 302)
		return
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
	if attachmentURL.String() != "" {
		if !contains(whitelist, strings.TrimPrefix(attachmentURL.Host, "www.")) {
			http.Redirect(w, r, "/battle/submit/unapprovedurl", 302)
			return
		}
	}

	attachment = policy.Sanitize(r.FormValue("attachment"))

	status := "entry"
	if r.FormValue("submit") == "DRAFT" {
		status = "draft"
	}

	battle := &Battle{
		Title:          strings.TrimSpace(policy.Sanitize(r.FormValue("title"))),
		Rules:          strings.TrimSpace(policy.Sanitize(r.FormValue("rules"))),
		Deadline:       deadline,
		VotingDeadline: votingDeadline,
		Attachment:     attachment,
		Host:           user.Name,
		Status:         status,
		Password:       policy.Sanitize(r.FormValue("password")),
		UserID:         user.ID,
		Entries:        0,
		ID:             0,
		MaxVotes:       maxVotes,
	}

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
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer ins.Close()

	var battleInsertedID int64 = 0
	res, err := ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment,
		battle.Status, battle.Password, user.ID, battle.VotingDeadline, battle.MaxVotes)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}

	battleInsertedID, err = res.LastInsertId()
	if err != nil {
		http.Redirect(w, r, "/successadd", 302)
		return
	}

	TagsDB(db, false, r.FormValue("tags"), battleInsertedID)

	http.Redirect(w, r, "/successadd", 302)
	return
}

// TagsDB adds / updates tags in the DB.
func TagsDB(db *sql.DB, update bool, tagsJSON string, battleID int64) {
	var tagIDs []int64

	// TODO - MIGHT BE SQL INJECTABLE OR SOMETHING

	var tags []Tag
	err := json.Unmarshal([]byte(tagsJSON), &tags)
	if err != nil {
		return
	}

	// Only accept 3 tags.
	for i, tag := range tags {
		if i > 2 {
			break
		}

		ins, err := db.Prepare("INSERT INTO tags(tag) VALUES(?) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)")
		if err != nil {
			return
		}
		defer ins.Close()

		res, err := ins.Exec(strings.TrimSpace(policy.Sanitize(tag.Value)))
		if err != nil {
			return
		}

		insertedID, err := res.LastInsertId()
		if err != nil {
			return
		}
		tagIDs = append(tagIDs, insertedID)
	}

	if update {
		del, err := db.Prepare("DELETE FROM challenges_tags WHERE challenge_id = ?")
		if err != nil {
			return
		}
		defer del.Close()

		del.Exec(battleID)
	}

	for i, tagID := range tagIDs {
		if i > 2 {
			break
		}

		ins, err := db.Prepare("INSERT INTO challenges_tags VALUES(?, ?)")
		if err != nil {
			return
		}
		defer ins.Close()

		ins.Exec(battleID, tagID)
	}
}

// GetTags retrieves tags from the DB
func GetTags(db *sql.DB, battleID int) []Tag {
	// TODO - TAGS (NEED TO GET INSERTED ID FOR BATTLE AND TAGS)
	// TODO - MIGHT BE SQL INJECTABLE OR SOMETHING
	var Tags []Tag

	query := `SELECT tags.tag FROM challenges_tags LEFT JOIN tags ON tags.id = challenges_tags.tag_id WHERE challenges_tags.challenge_id = ?`

	rows, err := db.Query(query, battleID)

	if err != nil {
		return Tags
	}
	defer rows.Close()

	for rows.Next() {
		tag := Tag{Value: ""}
		err = rows.Scan(&tag.Value)
		if err != nil {
			return Tags
		}

		tag.Value = html.UnescapeString(tag.Value)
		Tags = append(Tags, tag)
	}

	return Tags
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
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer ins.Close()

	ins.Exec(user.ID, battleID)

	http.Redirect(w, r, "/successdel", 302)
	return
}
