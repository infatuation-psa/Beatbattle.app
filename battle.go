package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
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
	Host           string    `gorm:"column:host" json:"host" validate:"required"`
	Status         string    `gorm:"column:status" json:"status" validate:"required"`
	Password       string    `gorm:"column:password" json:"password"`
	UserID         string    `gorm:"column:user_id" json:"user_id" validate:"required"`
	Entries        int       `json:"entries"`
	ChallengeID    int       `gorm:"column:challenge_id" json:"challenge_id"`
	MaxVotes       int       `gorm:"column:maxvotes" json:"maxvotes" validate:"required"`
}

// ViewBattles - Retrieves all battles and displays to user. Homepage.
func ViewBattles(res http.ResponseWriter, req *http.Request) {
	db := dbConn()
	defer db.Close()
	rows, err := db.Query("SELECT * FROM challenges ORDER BY challenge_id DESC")
	if err != nil {
		panic(err.Error())
	}

	defer rows.Close()
	battle := Battle{}
	challenges := []Battle{}
	for rows.Next() {
		err = rows.Scan(&battle.ChallengeID, &battle.Title, &battle.Rules, &battle.Deadline, &battle.Attachment, &battle.Host, &battle.Status,
			&battle.Password, &battle.UserID, &battle.VotingDeadline, &battle.MaxVotes)

		if err != nil {
			panic(err.Error())
		}

		switch battle.Status {
		case "entry":
			battle.Status = ParseDeadline(db, battle.Deadline, battle.ChallengeID, "entry", true)
		case "voting":
			battle.Status = ParseDeadline(db, battle.VotingDeadline, battle.ChallengeID, "voting", true)
		default:
			battle.Status = "Battle Finished" // Complete case
		}

		entries := 0
		err = db.QueryRow("SELECT COUNT(beat_id) FROM beats WHERE challenge_id=?", battle.ChallengeID).Scan(&entries)
		if err != nil && err != sql.ErrNoRows {
			panic(err.Error())
		}
		battle.Entries = entries

		challenges = append(challenges, battle)
	}

	challengesJSON, err := json.Marshal(challenges)
	if err != nil {
		fmt.Println(err)
		return
	}

	var user = GetUser(res, req)

	m := map[string]interface{}{
		"Challenges": string(challengesJSON),
		"User":       user,
	}

	tmpl.ExecuteTemplate(res, "Index", m)
}

// ViewBattle - Retreives battle and displays to user.
func ViewBattle(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	battleID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/", 301)
	}

	// Retrieve battle, return to front page if battle doesn't exist.
	battle := GetBattle(db, "SELECT * FROM challenges WHERE challenge_id = ?", battleID)
	if battle.Title == "" {
		http.Redirect(w, r, "/", 301)
		return
	}

	// Get beats user has voted for
	var user = GetUser(w, r)
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

	rows, err := db.Query("SELECT * FROM beats WHERE challenge_id = ? "+order, battleID)
	if err != nil {
		panic(err.Error())
	}
	defer rows.Close()

	submission := Beat{}
	entries := []Beat{}

	for rows.Next() {
		err = rows.Scan(&submission.ID, &submission.Discord, &submission.Artist, &submission.URL, &submission.Votes, &submission.ChallengeID, &submission.UserID)
		if err != nil {
			panic(err.Error())
		}

		count++

		submission.Color = "black"
		if binarySearch(submission.ID, lastVotes) {
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

	m := map[string]interface{}{
		"Battle":        battle,
		"Beats":         string(e),
		"User":          user,
		"EnteredBattle": hasEntered,
	}

	tmpl.ExecuteTemplate(w, "Battle", m)
}

// ParseDeadline returns a human readable deadline & updates the battle status in the database.
func ParseDeadline(db *sql.DB, deadline time.Time, battleID int, deadlineType string, shortForm bool) string {
	var deadlineParsed string = "Open - "
	var curStatus string

	err := db.QueryRow("SELECT status FROM challenges WHERE challenge_id = ?", battleID).Scan(&curStatus)
	if err != nil {
		panic(err.Error())
	}

	if time.Until(deadline) < 0 && curStatus == deadlineType {
		deadlineParsed = "Voting - "
		sql := "UPDATE challenges SET status = 'voting' WHERE challenge_id = ?"

		if curStatus == "voting" {
			deadlineParsed = "Battle Finished"
			sql = "UPDATE challenges SET status = 'complete' WHERE challenge_id = ?"
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

// GetBattle retrieves a battle from the database using an ID.
func GetBattle(db *sql.DB, sqlStmt string, battleID int) Battle {
	battle := Battle{}
	err := db.QueryRow("SELECT * FROM challenges WHERE challenge_id = ?", battleID).Scan(&battle.ChallengeID,
		&battle.Title, &battle.Rules, &battle.Deadline, &battle.Attachment, &battle.Host, &battle.Status,
		&battle.Password, &battle.UserID, &battle.VotingDeadline, &battle.MaxVotes)

	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	if err == sql.ErrNoRows {
		return battle
	}

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
	var user = GetUser(w, r)
	tmpl.ExecuteTemplate(w, "SubmitBattle", user)
}

// InsertBattle ...
func InsertBattle(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/auth/discord", 301)
		return
	}

	entries := 0
	err := db.QueryRow("SELECT COUNT(challenge_id) FROM challenges WHERE status=? AND user_id=?", "entry", user.ID).Scan(&entries)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	if entries >= 3 {
		println(entries)
		http.Redirect(w, r, "/", 301)
		return
	}

	// For time.Parse
	layout := "2006-01-02T15:04"
	deadline, err := time.Parse(layout, r.FormValue("deadline"))
	if err != nil || deadline.Before(time.Now()) {
		http.Redirect(w, r, "/", 301)
		return
	}

	votingDeadline, err := time.Parse(layout, r.FormValue("votingdeadline"))
	if err != nil || votingDeadline.Before(deadline) {
		http.Redirect(w, r, "/", 301)
		return
	}

	maxVotes, err := strconv.Atoi(r.FormValue("maxvotes"))
	if err != nil || maxVotes < 1 || maxVotes > 10 {
		http.Redirect(w, r, "/", 301)
		return
	}

	battle := &Battle{
		Title:          r.FormValue("title"),
		Rules:          r.FormValue("rules"),
		Deadline:       deadline,
		VotingDeadline: votingDeadline,
		Attachment:     r.FormValue("attachment"),
		Host:           user.Name,
		Status:         "entry",
		Password:       r.FormValue("password"),
		UserID:         user.ID,
		Entries:        0,
		ChallengeID:    0,
		MaxVotes:       maxVotes,
	}

	if user.Authenticated && r.Method == "POST" {
		v := validator.New()
		err = v.Struct(battle)

		if err != nil {
			for _, err := range err.(validator.ValidationErrors) {
				fmt.Println(err.Namespace())
			}
			http.Redirect(w, r, "/", 301)
			return
		}

		if RowExists(db, "SELECT challenge_id FROM challenges WHERE user_id = ? AND title = ?", user.ID, battle.Title) {
			http.Redirect(w, r, "/", 301)
			return
		}

		stmt := "INSERT INTO challenges(title, rules, deadline, attachment, host, status, password, user_id, voting_deadline, maxvotes) VALUES(?,?,?,?,?,?,?,?,?,?)"

		ins, err := db.Prepare(stmt)
		if err != nil {
			panic(err.Error())
		}
		defer ins.Close()

		fmt.Println(battle)

		ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment, user.Name,
			battle.Status, battle.Password, user.ID, battle.VotingDeadline, battle.MaxVotes)
	} else {
		print("Not post")
		http.Redirect(w, r, "/", 301)
		return
	}
	// TODO - Redirect with alert for user.
	http.Redirect(w, r, "/", 301)
	return
}
