package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator"
	"github.com/gomarkdown/markdown"
	"github.com/labstack/echo/v4"
)

// Battle ...
type Battle struct {
	// TODO - ADD PRIVATE BATTLE
	ID             int            `gorm:"column:id" json:"id"`
	Title          string         `gorm:"column:title" json:"title" validate:"required"`
	Rules          string         `gorm:"column:rules" validate:"required"`
	RulesHTML      template.HTML  `json:"rules"`
	Deadline       time.Time      `gorm:"column:deadline" json:"deadline" validate:"required"`
	VotingDeadline time.Time      `gorm:"column:voting_deadline" json:"voting_deadline" validate:"required"`
	Attachment     string         `gorm:"column:attachment" json:"attachment"`
	Status         string         `gorm:"column:status" json:"status"`
	ParsedDeadline string         `json:"parsed_deadline"`
	Password       string         `gorm:"column:password" json:"password"`
	Host           User           `json:"host"`
	Entries        int            `json:"entries"`
	MaxVotes       int            `gorm:"column:maxvotes" json:"maxvotes" validate:"required"`
	Type           string         `gorm:"column:type" json:"type"`
	Tags           []string       `json:"tags"`
	Results        int            `json:"results"`
	Settings       BattleSettings `json:"settings"`
}

type BattleSettings struct {
	ID          int    `gorm:"column:settings_id" json:"id"`
	Logo        string `gorm:"column:logo" json:"logo"`
	Background  string `gorm:"column:background" json:"background"`
	ShowUsers   bool   `gorm:"column:show_users" json:"show_users"`
	ShowEntries bool   `gorm:"column:show_entries" json:"show_entries"`
	TrackingID  string `gorm:"column:tracking_id" json:"tracking_id"`
	Private     bool   `gorm:"column:private" json:"private"`
	Field1      string `gorm:"column:field_1" json:"field_1"`
	Field2      string `gorm:"column:field_2" json:"field_2"`
	Field3      string `gorm:"column:field_3" json:"field_3"`
}

// ParseDeadline returns a human readable deadline & updates the battle status in the database.
func ParseDeadline(deadline time.Time, votingDeadline time.Time, battleID int, shortForm bool, homePage bool) string {
	var status string = "entry"
	var results int

	err := dbRead.QueryRow("SELECT results FROM battles WHERE id = ?", battleID).Scan(&results)
	if err != nil {
		log.Println(err)
		return ""
	}

	if results == -1 {
		status = "draft"
		return status
	}

	// If deadline has passed and status matches parameter
	// Adjust status in DB.
	if time.Until(deadline) < 0 {
		status = "voting"

		if time.Until(votingDeadline) < 0 {
			status = "complete"
			if results != 1 {
				err = BattleResults(battleID)
				if err != nil {
					log.Println(err)
				}

				updateResults, err := dbWrite.Prepare("UPDATE battles SET results = '1' WHERE id = ?")
				if err != nil {
					log.Println(err)
					return ""
				}
				defer updateResults.Close()
				updateResults.Exec(battleID)
			}
		}
	}
	return status
}

// ViewBattles - Retrieves all battles and displays to user. Homepage.
func ViewBattles(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	toast := GetToast(c)
	ads := GetAdvertisements()

	// Default to index
	tpl := "Index"
	title := "Who's The Best Producer?"
	status := "open"
	URL := c.Request().URL.String()
	if strings.Contains(URL, "past") {
		tpl = "Past"
		title = "Past Battles"
		status = "complete"
	}

	// Get battle & user data
	battles := GetBattles("status:" + status)
	battlesJSON, _ := json.Marshal(battles)
	me := GetUser(c, false)

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":     "Beat Battle - " + title,
			"Analytics": analyticsKey,
		},
		"Battles": string(battlesJSON),
		"Me":      me,
		"Toast":   toast,
		"Ads":     ads,
	}

	return c.Render(http.StatusOK, tpl, m)
}

// ViewTaggedBattles - Retrieves all tagged battles and displays to user.
func ViewTaggedBattles(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, false)
	toast := GetToast(c)
	ads := GetAdvertisements()

	// Do GetBattles("tag:value")
	title := "Battles Tagged With " + policy.Sanitize(c.Param("tag"))
	battles := GetBattles("tag:" + policy.Sanitize(c.Param("tag")))
	activeTag := policy.Sanitize(c.Param("tag"))
	battlesJSON, _ := json.Marshal(battles)

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":     "Beatbattle.app - " + title,
			"Analytics": analyticsKey,
		},
		"Battles": string(battlesJSON),
		"Me":      me,
		"Toast":   toast,
		"Tag":     activeTag,
		"Ads":     ads,
	}

	return c.Render(http.StatusOK, "ViewBattles", m)
}

// GetBattles retrieves battles from the database using a field and value.
// TODO - This is really messy. Think about splitting up the parts into each part of the query and combining.
func GetBattles(filter string) []Battle {

	start := time.Now()
	// FIELD & VALUE
	// TODO - SET UP PROPER USER GETTING
	query := `SELECT battles.id, battles.title, battles.deadline, battles.voting_deadline, 
			battles.type, battles.results, battles.tags, COUNT(DISTINCT beats.id) as entry_count,
			users.id, users.nickname, users.flair, IFNULL(battle_settings.private, 0)
			FROM battles
			LEFT JOIN users ON users.id = battles.user_ID
			LEFT JOIN beats ON battles.id = beats.battle_id
			LEFT JOIN battle_settings ON battle_settings.id = battles.settings_id`

	where := ""
	filterParams := strings.Split(filter, ":")
	switch filterParams[0] {
	case "status":
		switch filterParams[1] {
		case "complete":
			where = "WHERE results = 1"
		case "open":
			where = "WHERE results = 0 AND IFNULL(battle_settings.private, 0) = 0"
		}
	case "user":
		where = "WHERE battles.user_id = '" + policy.Sanitize(filterParams[1]) + "'"
	case "tag":
		// PERF: Like query is heavy.
		where = "WHERE battles.tags LIKE '%" + policy.Sanitize(filterParams[1]) + "%' AND IFNULL(battle_settings.private, 0) = 0 "
	}
	log.Println(policy.Sanitize(filterParams[1]))

	query += " " + where + " " + `GROUP BY battles.id
								ORDER BY battles.deadline DESC`

	rows, err := dbRead.Query(query)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer rows.Close()

	tags := ""

	battle := Battle{}
	battles := []Battle{}
	for rows.Next() {
		err = rows.Scan(&battle.ID, &battle.Title, &battle.Deadline, &battle.VotingDeadline,
			&battle.Type, &battle.Results, &tags, &battle.Entries,
			&battle.Host.ID, &battle.Host.Name, &battle.Host.Flair,
			&battle.Settings.Private)
		if err != nil {
			log.Println(err)
			return nil
		}

		battle.Title = html.UnescapeString(battle.Title)
		battle.Status = ParseDeadline(battle.Deadline, battle.VotingDeadline, battle.ID, true, true)
		battle.Tags = SetTags(tags)
		deadlineString := ""

		if battle.Status == "entry" {
			deadlineString += strconv.Itoa(int(battle.Deadline.UnixNano() / 1000000))
		}

		if battle.Status == "voting" {
			deadlineString += strconv.Itoa(int(battle.VotingDeadline.UnixNano() / 1000000))
		}

		if battle.Status == "complete" {
			layoutUS := "01/02/06"
			deadlineString += battle.VotingDeadline.Format(layoutUS)
		}

		battle.ParsedDeadline = deadlineString
		battles = append(battles, battle)
	}

	// Err handle (see other examples, might not be necessary)
	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	duration := time.Since(start)
	fmt.Println("GetBattles time: " + duration.String())

	return battles
}

// BattleResults updates the voted and votes columns of a battle.
func BattleResults(battleID int) error {
	start := time.Now()
	log.Println("test")
	sql := `UPDATE beats
			LEFT JOIN (SELECT beat_id, COUNT(beat_id) as beat_votes FROM votes WHERE battle_id = ? GROUP BY beat_id) beat_votes
				ON beat_votes.beat_id = beats.id
			LEFT JOIN (SELECT DISTINCT user_id, IF(user_id IS NOT NULL, true, false) as user_voted FROM votes WHERE battle_id = ? GROUP BY user_id) user_votes
				ON user_votes.user_id = beats.user_id
			SET
				beats.votes = IFNULL(beat_votes, 0),
				beats.voted = IFNULL(user_voted, FALSE)
			WHERE beats.battle_id = ?`

	upd, err := dbWrite.Prepare(sql)
	if err != nil {
		return err
	}
	defer upd.Close()

	upd.Exec(battleID, battleID, battleID)
	if err != nil {
		return err
	}

	sql = `UPDATE beats target
			JOIN
			(
				SELECT id, (@rownumber := @rownumber + 1) as rownum
				FROM beats         
				CROSS JOIN (SELECT @rownumber := 0) r
					WHERE beats.battle_id = ? AND beats.voted = 1
				ORDER BY votes DESC
			) source ON target.id = source.id    
			SET placement = rownum`

	placement, err := dbWrite.Prepare(sql)
	if err != nil {
		return err
	}
	defer upd.Close()

	placement.Exec(battleID)
	if err != nil {
		return err
	}

	duration := time.Since(start)
	fmt.Println("BattleResults time: " + duration.String())

	return nil
}

// BattleHTTP - Retrieves battle and displays to user.
// TODO - fix feedback
// TODO - slows down on the mainpage
func BattleHTTP(c echo.Context) error {
	start := time.Now()
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	toast := GetToast(c)
	ads := GetAdvertisements()

	// Validate that ID is an int.
	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Println(err)
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// Retrieve battle, return to front page if battle doesn't exist.
	battle := GetBattle(battleID)
	if battle.Title == "" {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// Get user's liked beats.
	var lastVotes []int
	var lastLikes []int
	me := GetUser(c, false)
	if me.Authenticated {
		likes, err := dbRead.Query("SELECT beat_id FROM likes WHERE user_id = ? AND battle_id = ? ORDER BY beat_id", me.ID, battleID)
		if err != nil && err != sql.ErrNoRows {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		defer likes.Close()

		for likes.Next() {
			var curBeatID int
			err = likes.Scan(&curBeatID)
			if err != nil {
				SetToast(c, "502")
				return c.Redirect(302, "/")
			}
			lastLikes = append(lastLikes, curBeatID)
		}
	}

	// Get beats user has voted for if in voting stage.
	if battle.Status == "voting" && me.Authenticated {
		votes, err := dbRead.Query("SELECT beat_id FROM votes WHERE user_id = ? AND battle_id = ? ORDER BY beat_id", me.ID, battleID)
		if err != nil && err != sql.ErrNoRows {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		defer votes.Close()

		for votes.Next() {
			var curBeatID int
			err = votes.Scan(&curBeatID)
			if err != nil {
				SetToast(c, "502")
				return c.Redirect(302, "/")
			}
			lastVotes = append(lastVotes, curBeatID)
		}
	}

	var count int
	entries := []Beat{}
	likes := []Beat{}
	didntVote := []Beat{}
	submission := Beat{}

	query := `SELECT 
			users.id, users.provider, users.provider_id, users.nickname, users.flair,
			beats.id, beats.url, beats.votes, beats.voted, beats.placement, IFNULL(feedback.feedback, ''),
			beats.field_1, beats.field_2, beats.field_3
			FROM beats
			LEFT JOIN users ON beats.user_id = users.id
			LEFT JOIN feedback ON feedback.user_id=? AND feedback.beat_id=beats.id
			WHERE beats.battle_id = ?
			GROUP BY 1`
	scanArgs := []interface{}{
		// Artist
		&submission.Artist.ID, &submission.Artist.Provider, &submission.Artist.ProviderID,
		&submission.Artist.Name, &submission.Artist.Flair,
		// Beat
		&submission.ID, &submission.URL, &submission.Votes,
		&submission.Voted, &submission.Placement, &submission.Feedback,
		&submission.Field1, &submission.Field2, &submission.Field3}

	rows, err := dbRead.Query(query, me.ID, battleID)
	if err != nil {
		log.Println(err)
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	entryPosition := 0
	hasEntered := false
	userVotes := 0

	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			log.Println(err)
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		count++

		if submission.Placement == 0 {
			submission.Placement = 999
		}
		submission.BattleID = battle.ID

		submission.UserVote = 0
		if battle.Status == "voting" {
			if ContainsInt(lastVotes, submission.ID) {
				submission.UserVote = 1
				if battle.Status == "complete" && submission.Artist.ID == me.ID {
					userVotes++
				}
				if battle.Status != "complete" {
					userVotes++
				}
			}
		}

		if battle.Status == "complete" && !submission.Voted {
			didntVote = append(didntVote, submission)
			if submission.Artist.ID == me.ID {
				hasEntered = true
				entryPosition = len(didntVote)
			}
			continue
		}

		submission.UserLike = 0
		if ContainsInt(lastLikes, submission.ID) {
			submission.UserLike = 1
			likes = append(likes, submission)
		}

		entries = append(entries, submission)
		if submission.Artist.ID == me.ID {
			hasEntered = true
			entryPosition = len(entries)
		}
	}

	// Handle if rows error exists, or if closing results in error.
	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	isOwner := me.ID == battle.Host.ID

	// Get user vote position.
	// TODO - Make this a function that is called from the client.
	if hasEntered && battle.Status == "voting" {
		query := `SELECT count(*)+1
					FROM beats
					WHERE battle_id=? AND votes > (SELECT votes FROM beats WHERE user_id=? AND battle_id=?)`
		dbRead.QueryRow(query, battleID, me.ID, battleID).Scan(&entryPosition)
	}

	if hasEntered && battle.Status == "complete" && userVotes == 0 {
		entryPosition += len(entries)
	}

	entries = append(entries, didntVote...)
	battle.Entries = count

	// Shuffle entries per user.
	if battle.Status != "complete" {
		rand.Seed(int64(me.ID * battle.ID))
		rand.Shuffle(len(entries), func(i, j int) {
			entries[i], entries[j] = entries[j], entries[i]
		})
	}

	filter := c.QueryParam("filter")
	if filter == "likes" {
		entries = likes
	}

	if battle.Status == "complete" {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Placement < entries[j].Placement
		})
	}

	// Convert the entries to JSON.
	e, err := json.Marshal(entries)
	if err != nil {
		log.Fatal(err)
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":     battle.Title,
			"Analytics": analyticsKey,
			"Buttons":   "Battle",
		},
		"Battle":         battle,
		"Beats":          string(e),
		"Me":             me,
		"EnteredBattle":  hasEntered,
		"EntryPosition":  entryPosition,
		"IsOwner":        isOwner,
		"Toast":          toast,
		"VotesRemaining": battle.MaxVotes - userVotes,
		"Ads":            ads,
		"Filter":         filter,
	}

	duration := time.Since(start)
	fmt.Println("BattleHTTP time: " + duration.String())

	return c.Render(http.StatusOK, "Battle", m)
}

// GetBattle retrieves a battle from the database using an ID.
func GetBattle(battleID int) Battle {
	start := time.Now()
	battle := Battle{}
	query := `
			SELECT users.id, users.nickname, users.flair, 
			battles.id, battles.title, battles.rules, battles.deadline, battles.voting_deadline, 
			battles.attachment, battles.password, battles.maxvotes, battles.type, battles.tags,
			battles.settings_id, IFNULL(battle_settings.logo, ''), IFNULL(battle_settings.background, ''),
			IFNULL(battle_settings.show_users, 0), IFNULL(battle_settings.show_entries, 0), 
			IFNULL(battle_settings.tracking_id, ""), IFNULL(battle_settings.private, 0), 
			IFNULL(battle_settings.field_1, ''), IFNULL(battle_settings.field_2, ''),
			IFNULL(battle_settings.field_3, '')
			FROM battles
			INNER JOIN users ON users.id = battles.user_id
			LEFT JOIN battle_settings ON battle_settings.id = battles.settings_id
			WHERE battles.id = ?`

	tags := ""
	err := dbRead.QueryRow(query, battleID).Scan(
		// Battle Host
		&battle.Host.ID, &battle.Host.Name, &battle.Host.Flair,
		// Battle
		&battle.ID, &battle.Title, &battle.Rules, &battle.Deadline, &battle.VotingDeadline,
		&battle.Attachment, &battle.Password, &battle.MaxVotes, &battle.Type, &tags,
		&battle.Settings.ID, &battle.Settings.Logo, &battle.Settings.Background,
		&battle.Settings.ShowUsers, &battle.Settings.ShowEntries,
		&battle.Settings.TrackingID, &battle.Settings.Private,
		&battle.Settings.Field1, &battle.Settings.Field2,
		&battle.Settings.Field3)
	if err != nil {
		log.Println(err)
		return battle
	}

	battle.Title = html.UnescapeString(battle.Title)
	md := []byte(html.UnescapeString(battle.Rules))
	battle.Rules = html.UnescapeString(battle.Rules)
	battle.RulesHTML = template.HTML(markdown.ToHTML(md, nil, nil))
	battle.Status = ParseDeadline(battle.Deadline, battle.VotingDeadline, battle.ID, true, false)
	battle.Tags = SetTags(tags)
	battle.Type = strings.Title(battle.Type)

	// Create parsed deadline.
	deadlineString := ""
	switch battle.Status {
	case "entry":
		deadlineString += strconv.Itoa(int(battle.Deadline.UnixNano() / 1000000))
	case "voting":
		deadlineString += strconv.Itoa(int(battle.VotingDeadline.UnixNano() / 1000000))
	default:
		layoutUS := "01/02/06"
		deadlineString += battle.VotingDeadline.Format(layoutUS)
	}
	battle.ParsedDeadline = deadlineString

	duration := time.Since(start)
	fmt.Println("GetBattle time: " + duration.String())
	return battle
}

// SubmitBattle ...
func SubmitBattle(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, false)
	fmt.Println(me)
	if !me.Authenticated {
		fmt.Println("Submit error")
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	toast := GetToast(c)
	ads := GetAdvertisements()

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":     "Submit Battle",
			"Analytics": analyticsKey,
		},
		"Me":    me,
		"Toast": toast,
		"Ads":   ads,
	}

	return c.Render(http.StatusOK, "SubmitBattle", m)
}

// UpdateBattle ...
func UpdateBattle(c echo.Context) error {
	start := time.Now()
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, false)

	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	ads := GetAdvertisements()
	toast := GetToast(c)
	region := c.Param("region")
	country := c.Param("country")

	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	loc, err := time.LoadLocation(policy.Sanitize(region + "/" + country))
	if err != nil {
		log.Println(err)
		loc, _ = time.LoadLocation("America/Toronto")
	}

	battle := GetBattle(battleID)
	if battle.Title == "" {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	if battle.Host.ID != me.ID {
		SetToast(c, "403")
		return c.Redirect(302, "/")
	}

	// For time.Parse
	layout := "Jan 2, 2006-03:04 PM"
	deadline := strings.Split(battle.Deadline.In(loc).Format(layout), "-")
	votingDeadline := strings.Split(battle.VotingDeadline.In(loc).Format(layout), "-")

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":     "Update Battle",
			"Analytics": analyticsKey,
		},
		"Title":              "Update Battle",
		"Battle":             battle,
		"Me":                 me,
		"DeadlineDate":       deadline[0],
		"DeadlineTime":       deadline[1],
		"VotingDeadlineDate": votingDeadline[0],
		"VotingDeadlineTime": votingDeadline[1],
		"Toast":              toast,
		"Ads":                ads,
	}

	duration := time.Since(start)
	fmt.Println("UpdateBattle time: " + duration.String())
	return c.Render(http.StatusOK, "UpdateBattle", m)
}

// UpdateBattleDB ...
// TODO - Return to battle ID.
func UpdateBattleDB(c echo.Context) error {
	start := time.Now()
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated, if not kick them out.
	me := GetUser(c, true)
	if !me.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, true, "/", "502")
	}

	// Check if battle type is valid.
	battleType := policy.Sanitize(c.FormValue("type"))
	if battleType != "beat" && battleType != "rap" && battleType != "art" {
		return AjaxResponse(c, false, "/battle/submit", "invalidtype")
	}

	// Check if user owns battle
	userID := -1
	err = dbRead.QueryRow("SELECT user_id FROM battles WHERE id = ?", battleID).Scan(&userID)
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, true, "/", "502")
	}
	if userID != me.ID {
		return AjaxResponse(c, true, "/", "403")
	}

	// Handle time localization and deadline parsing.
	loc, err := time.LoadLocation(policy.Sanitize(c.FormValue("timezone")))
	if err != nil {
		log.Println(err)
		loc, _ = time.LoadLocation("America/Toronto")
	}

	// Parse Deadlines
	layout := "Jan 2, 2006 03:04 PM"
	unparsedDeadline := policy.Sanitize(c.FormValue("deadline-date") + " " + c.FormValue("deadline-time"))
	deadline, err := time.ParseInLocation(layout, unparsedDeadline, loc)
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, false, "/battle/"+c.Param("id")+"/update", "502")
	}
	unparsedVotingDeadline := policy.Sanitize(c.FormValue("votingdeadline-date") + " " + c.FormValue("votingdeadline-time"))
	votingDeadline, err := time.ParseInLocation(layout, unparsedVotingDeadline, loc)
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, false, "/battle/"+c.Param("id")+"/update", "502")
	}
	if votingDeadline.Before(deadline) {
		log.Println(err)
		return AjaxResponse(c, false, "/battle/"+c.Param("id")+"/update", "voteb4")
	}

	attachment := policy.Sanitize(c.FormValue("attachment"))
	maxVotes, err := strconv.Atoi(policy.Sanitize(c.FormValue("maxvotes")))
	if err != nil {
		maxVotes = 3
		log.Println(err)
		return AjaxResponse(c, false, "/battle/"+c.Param("id")+"/update", "502")
	}

	battle := &Battle{
		Title:          policy.Sanitize(c.FormValue("title")),
		Rules:          policy.Sanitize(c.FormValue("rules")),
		Deadline:       deadline,
		VotingDeadline: votingDeadline,
		Attachment:     attachment,
		Host:           me,
		Password:       policy.Sanitize(c.FormValue("password")),
		MaxVotes:       maxVotes,
		Type:           battleType,
	}

	// Validate the struct. This might be unnecessary.
	v := validator.New()
	err = v.Struct(battle)
	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			fmt.Println(err.Namespace())
		}
		return AjaxResponse(c, false, "/battle/"+c.Param("id")+"/update", "validationerror")
	}

	logo := policy.Sanitize(c.FormValue("logo"))
	background := policy.Sanitize(c.FormValue("background"))
	showUsers, _ := strconv.Atoi(policy.Sanitize(c.FormValue("show_users")))
	showEntries, _ := strconv.Atoi(policy.Sanitize(c.FormValue("show_entries")))
	trackingID := policy.Sanitize(c.FormValue("tracking_id"))
	private, _ := strconv.Atoi(policy.Sanitize(c.FormValue("private")))
	field1 := policy.Sanitize(c.FormValue("field_1"))
	field2 := policy.Sanitize(c.FormValue("field_2"))
	field3 := policy.Sanitize(c.FormValue("field_3"))

	// If style ID exists, update. Otherwise, insert.
	settingsID, _ := strconv.Atoi(policy.Sanitize(c.FormValue("settings_id")))
	log.Println(settingsID)
	if settingsID == 0 {
		if len(logo) > 0 || len(background) > 0 ||
			showUsers == 1 || showEntries == 1 ||
			len(trackingID) > 0 || private == 1 ||
			len(field1) > 0 || len(field2) > 0 ||
			len(field3) > 0 {
			stmt := "INSERT INTO battle_settings(logo, background, show_users, show_entries, tracking_id, private, field_1, field_2, field_3) VALUES(?,?,?,?,?,?,?,?,?)"
			ins, err := dbWrite.Prepare(stmt)
			if err != nil {
				log.Println(err)
				return AjaxResponse(c, false, "/battle/submit", "502")
			}
			defer ins.Close()

			res, err := ins.Exec(logo, background, showUsers, showEntries, trackingID, private, field1, field2, field3)
			if err != nil {
				log.Println(err)
				SetToast(c, "502")
				return c.Redirect(302, "/")
			}
			lastInsertID, _ := res.LastInsertId()
			settingsID = int(lastInsertID) // truncated on machines with 32-bit ints
		}
	} else {
		stmt := "UPDATE battle_settings SET logo = ?, background = ?, show_users = ?, show_entries = ?, tracking_id = ?, private = ?, field_1 = ?, field_2 = ?, field_3 = ? WHERE id = ?"
		upd, err := dbWrite.Prepare(stmt)
		if err != nil {
			log.Println(err)
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		defer upd.Close()
		upd.Exec(logo, background, showUsers, showEntries, trackingID, private, field1, field2, field3, settingsID)
	}

	results := 0
	if c.FormValue("submit") == "DRAFT" {
		results = -1
	}

	query := `
			UPDATE battles 
			SET title = ?, rules = ?, deadline = ?, attachment = ?, password = ?, voting_deadline = ?, maxvotes = ?, type = ?, settings_id = ?, results = ?, tags = ?
			WHERE id = ? AND user_id = ?`

	ins, err := dbWrite.Prepare(query)
	if err != nil {
		log.Println(err)
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer ins.Close()

	ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment,
		battle.Password, battle.VotingDeadline, battle.MaxVotes,
		battle.Type, settingsID, results, c.FormValue("tags"), battleID, me.ID)
	if err != nil {
		log.Println(err)
		SetToast(c, "failadd")
		return c.Redirect(302, "/")
	}

	SetToast(c, "successupdate")

	duration := time.Since(start)
	fmt.Println("UpdateBattleDB time: " + duration.String())

	return c.Redirect(302, "/")
}

// InsertBattle ...
func InsertBattle(c echo.Context) error {
	start := time.Now()
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated.
	me := GetUser(c, true)
	if !me.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	battleType := policy.Sanitize(c.FormValue("type"))
	if battleType != "beat" && battleType != "rap" && battleType != "art" {
		return AjaxResponse(c, false, "/battle/submit", "invalidtype")
	}

	// Handle time localization and deadline parsing.
	loc, err := time.LoadLocation(policy.Sanitize(c.FormValue("timezone")))
	if err != nil {
		log.Println(err)
		loc, _ = time.LoadLocation("America/Toronto")
	}

	// Parse Deadlines
	layout := "Jan 2, 2006 03:04 PM"
	unparsedDeadline := policy.Sanitize(c.FormValue("deadline-date") + " " + c.FormValue("deadline-time"))
	deadline, err := time.ParseInLocation(layout, unparsedDeadline, loc)
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, false, "/battle/submit", "502")
	}

	unparsedVotingDeadline := policy.Sanitize(c.FormValue("votingdeadline-date") + " " + c.FormValue("votingdeadline-time"))
	votingDeadline, err := time.ParseInLocation(layout, unparsedVotingDeadline, loc)
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, false, "/battle/submit", "502")
	}
	if votingDeadline.Before(deadline) {
		return AjaxResponse(c, false, "/battle/submit", "voteb4")
	}

	// TO DO - If max votes 0 then unlimited votes.
	attachment := policy.Sanitize(c.FormValue("attachment"))
	maxVotes, err := strconv.Atoi(policy.Sanitize(c.FormValue("maxvotes")))
	if err != nil {
		maxVotes = 3
		log.Println(err)
		return AjaxResponse(c, false, "/battle/submit", "502")
	}

	battle := &Battle{
		Title:          strings.TrimSpace(policy.Sanitize(c.FormValue("title"))),
		Rules:          strings.TrimSpace(policy.Sanitize(c.FormValue("rules"))),
		Deadline:       deadline,
		VotingDeadline: votingDeadline,
		Attachment:     attachment,
		Host:           me,
		Password:       policy.Sanitize(c.FormValue("password")),
		Entries:        0,
		ID:             0,
		MaxVotes:       maxVotes,
		Type:           battleType,
	}

	results := 0
	if c.FormValue("submit") == "DRAFT" {
		results = -1
	}

	v := validator.New()
	err = v.Struct(battle)

	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			fmt.Println(err.Namespace())
		}
		return AjaxResponse(c, false, "/battle/submit", "validationerror")
	}

	logo := policy.Sanitize(c.FormValue("logo"))
	background := policy.Sanitize(c.FormValue("background"))
	showUsers, _ := strconv.Atoi(policy.Sanitize(c.FormValue("show_users")))
	showEntries, _ := strconv.Atoi(policy.Sanitize(c.FormValue("show_entries")))
	trackingID := policy.Sanitize(c.FormValue("tracking_id"))
	private, _ := strconv.Atoi(policy.Sanitize(c.FormValue("private")))
	field1 := policy.Sanitize(c.FormValue("field_1"))
	field2 := policy.Sanitize(c.FormValue("field_2"))
	field3 := policy.Sanitize(c.FormValue("field_3"))

	// If style ID exists, update. Otherwise, insert.
	settingsID, _ := strconv.Atoi(policy.Sanitize(c.FormValue("settings_id")))
	log.Println(settingsID)
	if settingsID == 0 {
		if len(logo) > 0 || len(background) > 0 ||
			showUsers == 1 || showEntries == 1 ||
			len(trackingID) > 0 || private == 1 ||
			len(field1) > 0 || len(field2) > 0 ||
			len(field3) > 0 {
			stmt := "INSERT INTO battle_settings(logo, background, show_users, show_entries, tracking_id, private, field_1, field_2, field_3) VALUES(?,?,?,?,?,?,?,?,?)"
			ins, err := dbWrite.Prepare(stmt)
			if err != nil {
				log.Println(err)
				return AjaxResponse(c, false, "/battle/submit", "502")
			}
			defer ins.Close()

			res, err := ins.Exec(logo, background, showUsers, showEntries, trackingID, private, field1, field2, field3)
			if err != nil {
				log.Println(err)
				return AjaxResponse(c, false, "/battle/submit", "502")
			}
			lastInsertID, _ := res.LastInsertId()
			settingsID = int(lastInsertID) // truncated on machines with 32-bit ints
		}
	}

	stmt := `INSERT INTO battles
			(title, rules, results, deadline, attachment, password, user_id, 
			voting_deadline, maxvotes, type, settings_id, tags)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`
	ins, err := dbWrite.Prepare(stmt)
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, false, "/battle/submit", "502")
	}
	defer ins.Close()
	res, err := ins.Exec(battle.Title, battle.Rules, results, battle.Deadline, battle.Attachment, battle.Password,
		battle.Host.ID, battle.VotingDeadline, battle.MaxVotes, battle.Type, settingsID, c.FormValue("tags"))
	if err != nil {
		log.Println(err)
		return AjaxResponse(c, false, "/battle/submit", "502")
	}
	battleInsertedID, _ := res.LastInsertId()

	duration := time.Since(start)
	fmt.Println("InsertBattle time: " + duration.String())
	return AjaxResponse(c, true, "/battle/"+strconv.FormatInt(battleInsertedID, 10), "successadd")
}

// SetTags resolves a battle's tags.
func SetTags(tags string) []string {
	names := string(tags)
	tagValues := strings.Split(names, ",")
	if names == "" {
		tagValues = nil
	}

	return tagValues
}

// CloseBattle ...
func CloseBattle(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated.
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	// Get battle ID from the request.
	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// Check if the delete request was sent through the form.
	if c.FormValue("close") == "yes" {
		stmt := "UPDATE battles SET deadline = NOW() WHERE user_id = ? AND id = ?"

		del, err := dbWrite.Prepare(stmt)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		defer del.Close()
		del.Exec(me.ID, battleID)

		SetToast(c, "successclose")
		return c.Redirect(302, "/")
	}

	// Return not user by default.
	SetToast(c, "403")
	return c.Redirect(302, "/")
}

// DeleteBattle ...
func DeleteBattle(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated.
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	// Get battle ID from the request.
	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// Check if the delete request was sent through the form.
	if c.FormValue("delete") == "yes" {
		stmt := "DELETE FROM battles WHERE user_id = ? AND id = ?"

		del, err := dbWrite.Prepare(stmt)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		defer del.Close()
		del.Exec(me.ID, battleID)

		SetToast(c, "successdel")
		return c.Redirect(302, "/")
	}

	// Return not user by default.
	SetToast(c, "403")
	return c.Redirect(302, "/")
}
