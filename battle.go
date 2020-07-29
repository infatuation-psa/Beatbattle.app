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
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator"
	"github.com/gomarkdown/markdown"
	"github.com/labstack/echo/v4"
)

// Battle ...
type Battle struct {
	ID             int           `gorm:"column:id" json:"id"`
	Title          string        `gorm:"column:title" json:"title" validate:"required"`
	Rules          string        `gorm:"column:rules" validate:"required"`
	RulesHTML      template.HTML `json:"rules"`
	Deadline       time.Time     `gorm:"column:deadline" json:"deadline" validate:"required"`
	VotingDeadline time.Time     `gorm:"column:voting_deadline" json:"voting_deadline" validate:"required"`
	Attachment     string        `gorm:"column:attachment" json:"attachment"`
	Status         string        `gorm:"column:status"`
	StatusDisplay  string        `json:"status"`
	Password       string        `gorm:"column:password" json:"password"`
	Host           User          `json:"host"`
	Entries        int           `json:"entries"`
	MaxVotes       int           `gorm:"column:maxvotes" json:"maxvotes" validate:"required"`
	GroupID        int           `gorm:"column:group_id" json:"group_id"`
	Type           string        `gorm:"column:type" json:"type"`
	Tags           []Tag
}

// Tag ...
type Tag struct {
	Value string `json:"tag"`
}

// ParseDeadline returns a human readable deadline & updates the battle status in the database.
func ParseDeadline(deadline time.Time, battleID int, deadlineType string, shortForm bool) string {
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

	if days > 0 {
		deadlineParsed += strconv.Itoa(days) + "d "
	}

	if hours > 0 {
		deadlineParsed += strconv.Itoa(hours) + "h "
	}

	if minutes > 0 {
		deadlineParsed += strconv.Itoa(minutes) + "m "
	} else {
		deadlineParsed += "1m "
	}

	if !shortForm {
		if curStatus == "entry" {
			deadlineParsed += "til voting starts"
		}

		if curStatus == "voting" {
			deadlineParsed += "til voting ends"
		}
	}

	return deadlineParsed
}

// ViewBattles - Retrieves all battles and displays to user. Homepage.
func ViewBattles(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	toast := GetToast(c)
	ads := GetAdvertisements()
	URL := c.Request().URL.String()

	tpl := "Index"
	status := "entry"
	title := "Who's The Best Producer?"
	if strings.Contains(URL, "past") {
		tpl = "Past"
		status = "complete"
		title = "Past Battles"
	}

	battles := GetBattles("challenges.status", status)
	battlesJSON, _ := json.Marshal(battles)
	me := GetUser(c, false)

	m := map[string]interface{}{
		"Title":   title,
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
	title := "Battles Tagged With " + policy.Sanitize(c.Param("tag"))
	battles := GetBattles("tags.tag", policy.Sanitize(c.Param("tag")))
	activeTag := policy.Sanitize(c.Param("tag"))
	battlesJSON, _ := json.Marshal(battles)

	m := map[string]interface{}{
		"Title":   title,
		"Battles": string(battlesJSON),
		"Me":      me,
		"Toast":   toast,
		"Tag":     activeTag,
		"Ads":     ads,
	}

	return c.Render(http.StatusOK, "ViewBattles", m)
}

// GetBattles retrieves battles from the database using a field and value.
func GetBattles(field string, value string) []Battle {
	// FIELD & VALUE
	querySELECT := `SELECT challenges.id, challenges.title, challenges.deadline, challenges.voting_deadline, challenges.status, challenges.user_id, challenges.type, COUNT(beats.id) as entry_count
					FROM challenges 
					LEFT JOIN beats ON challenges.id = beats.challenge_id`
	queryWHERE := "WHERE " + field + "=?"
	queryGROUP := "GROUP BY 1"
	queryORDER := "ORDER BY challenges.deadline DESC"

	if field == "tags.tag" {
		querySELECT = `SELECT challenges.id, challenges.title, challenges.deadline, challenges.voting_deadline, challenges.status, challenges.user_id, challenges.type, COUNT(beats.id) as entry_count
						FROM tags
						LEFT JOIN challenges_tags ON challenges_tags.tag_id = tags.id
						LEFT JOIN challenges on challenges.id = challenges_tags.challenge_id
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
		err = rows.Scan(&battle.ID, &battle.Title, &battle.Deadline, &battle.VotingDeadline, &battle.Status, &battle.Host.ID, &battle.Type, &battle.Entries)
		if err != nil {
			return nil
		}

		switch battle.Status {
		case "entry":
			battle.StatusDisplay = ParseDeadline(battle.Deadline, battle.ID, "entry", true)
		case "voting":
			battle.StatusDisplay = ParseDeadline(battle.VotingDeadline, battle.ID, "voting", true)
		case "draft":
			battle.StatusDisplay = "Draft"
		default:
			layoutUS := "January 2, 2006"
			battle.StatusDisplay = "Finished - " + battle.VotingDeadline.Format(layoutUS) // Complete case
		}

		battle.Host = GetUserDB(battle.Host.ID)
		battle.Tags = GetTags(battle.ID)
		battle.Type = strings.Title(battle.Type)
		battles = append(battles, battle)
	}

	// Err handle (see other examples, might not be necessary)
	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	return battles
}

// BattleHTTP - Retrieves battle and displays to user.
func BattleHTTP(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	toast := GetToast(c)
	ads := GetAdvertisements()

	// Validate that ID is an int.
	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// Retrieve battle, return to front page if battle doesn't exist.
	battle := GetBattle(battleID)
	if battle.Title == "" {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	me := GetUser(c, false)
	var lastVotes []int
	var lastLikes []int

	// Get beats user has liked.
	if me.Authenticated {
		likes, err := db.Query("SELECT beat_id FROM likes WHERE user_id = ? AND challenge_id = ? ORDER BY beat_id", me.ID, battleID)
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
		votes, err := db.Query("SELECT beat_id FROM votes WHERE user_id = ? AND challenge_id = ? ORDER BY beat_id", me.ID, battleID)
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
	didntVote := []Beat{}
	submission := Beat{}

	args := []interface{}{me.ID, battleID}
	scanArgs := []interface{}{&submission.ID, &submission.URL, &submission.Feedback, &submission.Artist.ID}
	query := `SELECT beats.id, beats.url, IFNULL(feedback.feedback, ''), beats.user_id
			FROM beats 
			LEFT JOIN feedback on feedback.user_id=? AND feedback.beat_id=beats.id
			WHERE beats.challenge_id=?
			GROUP BY 1
			ORDER BY beats.id DESC`

	if battle.Status == "complete" {
		query = `SELECT beats.id, beats.url, beats.votes, beats.user_id, beats.voted
				FROM beats 
				WHERE beats.challenge_id=?
				GROUP BY 1
				ORDER BY votes DESC`
		args = []interface{}{battleID}
		scanArgs = []interface{}{&submission.ID, &submission.URL, &submission.Votes, &submission.Artist.ID, &submission.Voted}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	ua := c.Request().Header.Get("User-Agent")
	mobileUA := regexp.MustCompile(`/Mobile|Android|BlackBerry|iPhone/`)
	isMobile := mobileUA.MatchString(ua)

	// PERF
	entryPosition := 0
	hasEntered := false
	userVotes := 0

	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		submission.Artist = GetUserDB(submission.Artist.ID)
		if battle.Status == "complete" && !submission.Voted {
			submission.Artist.NameHTML = submission.Artist.NameHTML + `&nbsp;<span class="tooltipped" style="color: #1E19FF;" data-tooltip="Did Not Vote">(*)</span>`
		}
		count++

		// VoteColour & LikeColour are workarounds to the limits of ZingGrid.
		// ZingGrid usage should be deprecated eventually.
		// Actually after 4 hours of research zinggrid isn't that bad compared to alternatives
		submission.VoteColour = ""
		if battle.Status == "voting" {
			if ContainsInt(lastVotes, submission.ID) {
				submission.VoteColour = "#ff5800"
				if battle.Status == "complete" && submission.Artist.ID == me.ID {
					userVotes++
				}
				if battle.Status != "complete" {
					userVotes++
				}
			}
		}

		submission.LikeColour = ""
		if ContainsInt(lastLikes, submission.ID) {
			submission.LikeColour = "#ff5800"
		}

		u, _ := url.Parse(submission.URL)
		urlSplit := strings.Split(u.RequestURI(), "/")

		width := "width='100%'"
		if battle.Status != "complete" {
			width = "width='20px'"
		}

		// Build the track URL.
		if len(urlSplit) >= 4 {
			secretURL := urlSplit[3]
			if strings.Contains(secretURL, "s-") {
				submission.URL = `<iframe ` + width + ` height='20' scrolling='no' frameborder='no' allow='autoplay' show_user='false' src='https://w.soundcloud.com/player/?url=https://soundcloud.com/` + urlSplit[1] + "/" + urlSplit[2] + `?secret_token=` + urlSplit[3] + `&color=%23ff5500&inverse=false&auto_play=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
			} else {
				submission.URL = `<iframe ` + width + ` height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&auto_play=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
			}
		} else {
			submission.URL = `<iframe ` + width + ` height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&auto_play=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
		}

		if battle.Status == "complete" && !submission.Voted {
			didntVote = append(didntVote, submission)
			if submission.Artist.ID == me.ID {
				hasEntered = true
				entryPosition = len(didntVote)
			}
			continue
		}

		entries = append(entries, submission)
		if submission.Artist.ID == me.ID {
			hasEntered = true
			entryPosition = len(entries)
		}
	}

	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	if hasEntered && battle.Status == "voting" {
		query := `SELECT count(*)+1
					FROM beats
					WHERE challenge_id=? AND votes > (SELECT votes FROM beats WHERE user_id=? AND challenge_id=?)`

		db.QueryRow(query, battleID, me.ID, battleID).Scan(&entryPosition)
	}

	if hasEntered && battle.Status == "complete" && userVotes == 0 {
		entryPosition += len(entries)
	}

	entries = append(entries, didntVote...)

	battle.Entries = count

	// PERF - Shuffle entries per fcuser.
	if battle.Status != "complete" {
		rand.Seed(int64(me.ID * battle.ID))
		rand.Shuffle(len(entries), func(i, j int) {
			entries[i], entries[j] = entries[j], entries[i]
		})
	}

	e, err := json.Marshal(entries)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	isOwner := RowExists("SELECT id FROM challenges WHERE user_id = ? AND id = ?", me.ID, battleID)

	canEnter := true

	if battle.GroupID != 0 {
		canEnter = RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, battle.GroupID)
	}

	m := map[string]interface{}{
		"Title":          battle.Title,
		"Battle":         battle,
		"Beats":          string(e),
		"Me":             me,
		"CanEnter":       canEnter,
		"EnteredBattle":  hasEntered,
		"EntryPosition":  entryPosition,
		"IsOwner":        isOwner,
		"Toast":          toast,
		"IsMobile":       isMobile,
		"VotesRemaining": battle.MaxVotes - userVotes,
		"Ads":            ads,
	}

	return c.Render(http.StatusOK, "Battle", m)
}

// GetBattle retrieves a battle from the database using an ID.
func GetBattle(battleID int) Battle {
	battle := Battle{}

	query := `
		SELECT challenges.id, challenges.title, challenges.rules, challenges.deadline, challenges.voting_deadline, challenges.attachment, challenges.status, challenges.password, challenges.maxvotes, challenges.user_id, challenges.group_id, challenges.type
		FROM challenges 
        WHERE challenges.id = ?`

	err := db.QueryRow(query, battleID).Scan(&battle.ID,
		&battle.Title, &battle.Rules, &battle.Deadline, &battle.VotingDeadline, &battle.Attachment, &battle.Status,
		&battle.Password, &battle.MaxVotes, &battle.Host.ID, &battle.GroupID, &battle.Type)

	if err != nil {
		return battle
	}

	battle.Title = html.UnescapeString(battle.Title)
	battle.Host = GetUserDB(battle.Host.ID)

	md := []byte(html.UnescapeString(battle.Rules))
	battle.Rules = html.UnescapeString(battle.Rules)
	battle.RulesHTML = template.HTML(markdown.ToHTML(md, nil, nil))

	switch battle.Status {
	case "entry":
		battle.StatusDisplay = ParseDeadline(battle.Deadline, battleID, "entry", false)
	case "voting":
		battle.StatusDisplay = ParseDeadline(battle.VotingDeadline, battleID, "voting", false)
	case "draft":
		battle.StatusDisplay = "Draft"
	default:
		layoutUS := "January 2, 2006"
		battle.StatusDisplay = "Finished - " + battle.VotingDeadline.Format(layoutUS) // Complete case
	}

	battle.Tags = GetTags(battleID)
	battle.Type = strings.Title(battle.Type)

	return battle
}

// SubmitBattle ...
func SubmitBattle(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, false)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	toast := GetToast(c)
	ads := GetAdvertisements()

	userGroups := []Group{}
	if me.Authenticated {
		userGroups = GetGroupsByRole(db, me.ID, "member")
	}

	m := map[string]interface{}{
		"Title":      "Submit Battle",
		"Me":         me,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Ads":        ads,
	}

	return c.Render(http.StatusOK, "SubmitBattle", m)
}

// UpdateBattle ...
func UpdateBattle(c echo.Context) error {
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
		loc, _ = time.LoadLocation("America/Toronto")
	}

	battle := GetBattle(battleID)
	if battle.Title == "" {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	if battle.Host.ID != me.ID {
		SetToast(c, "notuser")
		return c.Redirect(302, "/")
	}

	userGroups := []Group{}
	if me.Authenticated {
		userGroups = GetGroupsByRole(db, me.ID, "member")
	}

	// For time.Parse
	layout := "Jan 2, 2006-03:04 PM"

	deadline := strings.Split(battle.Deadline.In(loc).Format(layout), "-")
	votingDeadline := strings.Split(battle.VotingDeadline.In(loc).Format(layout), "-")

	m := map[string]interface{}{
		"Title":              "Update Battle",
		"Battle":             battle,
		"Me":                 me,
		"UserGroups":         userGroups,
		"DeadlineDate":       deadline[0],
		"DeadlineTime":       deadline[1],
		"VotingDeadlineDate": votingDeadline[0],
		"VotingDeadlineTime": votingDeadline[1],
		"Toast":              toast,
		"Ads":                ads,
	}

	return c.Render(http.StatusOK, "UpdateBattle", m)
}

// UpdateBattleDB ...
func UpdateBattleDB(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated, if not kick them out.
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

	groupID, err := strconv.Atoi(policy.Sanitize(c.FormValue("group")))
	if err != nil {
		groupID = 0
	}

	// Check if user has permissions to modify battle. This check seems to be redundant?
	if groupID != 0 {
		if !RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, groupID) {
			SetToast(c, "notuser")
			return c.Redirect(302, "/battle/submit")
		}
	}

	// Check if battle type is valid.
	battleType := policy.Sanitize(c.FormValue("type"))
	if battleType != "beat" && battleType != "rap" {
		SetToast(c, "invalidtype")
		return c.Redirect(302, "/battle/submit")
	}

	// Check if battle is open and able to be edited. Checks if user ID matches the battle's ID.
	curStatus := "entry"
	userID := -1
	err = db.QueryRow("SELECT status, user_id FROM challenges WHERE id = ?", battleID).Scan(&curStatus, &userID)
	if err != nil || userID != me.ID {
		SetToast(c, "notuser")
		return c.Redirect(302, "/")
	}

	// Handle time localization and deadline parsing.
	loc, err := time.LoadLocation(policy.Sanitize(c.FormValue("timezone")))
	if err != nil {
		loc, _ = time.LoadLocation("America/Toronto")
	}

	// For time.Parse
	layout := "Jan 2, 2006 03:04 PM"

	unparsedDeadline := policy.Sanitize(c.FormValue("deadline-date") + " " + c.FormValue("deadline-time"))
	deadline, err := time.ParseInLocation(layout, unparsedDeadline, loc)
	if deadline.Before(time.Now()) {
		if curStatus == "entry" {
			SetToast(c, "deadb4")
			return c.Redirect(302, "/battle/"+c.Param("id")+"/update")
		}
	}

	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/battle/"+c.Param("id")+"/update")
	}

	unparsedVotingDeadline := policy.Sanitize(c.FormValue("votingdeadline-date") + " " + c.FormValue("votingdeadline-time"))
	votingDeadline, err := time.ParseInLocation(layout, unparsedVotingDeadline, loc)
	if err != nil || votingDeadline.Before(deadline) {
		SetToast(c, "voteb4")
		return c.Redirect(302, "/battle/"+c.Param("id")+"/update")
	}

	maxVotes, err := strconv.Atoi(policy.Sanitize(c.FormValue("maxvotes")))
	if err != nil || maxVotes < 1 || maxVotes > 10 {
		SetToast(c, "maxvotesinvalid")
		return c.Redirect(302, "/battle/"+c.Param("id")+"/update")
	}

	/* PERF - Removed URL whitelist temporarily.
	attachmentURL, err := url.Parse(policy.Sanitize(c.FormValue("attachment")))
	if err != nil {
		SetToast(c, "unapprovedurl")
		return c.Redirect(302, "/battle/"+c.Param("id")+"/update")
	}

	attachment := ""
	// PERF - MIGHT IMPACT A LOT*
		if attachmentURL.String() != "" {
			if !contains(whitelist, strings.TrimPrefix(attachmentURL.Host, "www.")) {
				SetToast(c, "unapprovedurl")
				return c.Redirect(302, "/battle/"+c.Param("id")+"/update")
			}
		}
	*/

	attachment := policy.Sanitize(c.FormValue("attachment"))

	// Determine if this is an update, pushing it live, or something else.
	status := curStatus
	if status == "draft" && c.FormValue("submit") == "PUBLISH" {
		status = "entry"
	}
	if c.FormValue("submit") == "DRAFT" {
		status = "draft"
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
		Status:         status,
		GroupID:        groupID,
		Type:           battleType,
	}

	// Validate the struct. This might be unnecessary.
	v := validator.New()
	err = v.Struct(battle)

	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			fmt.Println(err.Namespace())
		}
		SetToast(c, "validationerror")
		return c.Redirect(302, "/battle/"+c.Param("id")+"/update")
	}

	query := `
			UPDATE challenges 
			SET title = ?, rules = ?, deadline = ?, attachment = ?, password = ?, voting_deadline = ?, maxvotes = ?, status = ?, group_id = ?, type = ?
			WHERE id = ? AND user_id = ?`

	ins, err := db.Prepare(query)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer ins.Close()

	ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment, battle.Password, battle.VotingDeadline, battle.MaxVotes, battle.Status, battle.GroupID, battle.Type, battleID, me.ID)
	if err != nil {
		SetToast(c, "failadd")
		return c.Redirect(302, "/")
	}

	TagsDB(true, c.FormValue("tags"), int64(battleID))
	SetToast(c, "successupdate")
	return c.Redirect(302, "/")
}

// InsertBattle ...
func InsertBattle(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated.
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	entries := 0
	err := db.QueryRow("SELECT COUNT(id) FROM challenges WHERE status=? AND user_id=?", "entry", me.ID).Scan(&entries)
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	groupID, err := strconv.Atoi(policy.Sanitize(c.FormValue("group")))
	if err != nil {
		groupID = 0
	}

	if groupID != 0 {
		hasPermissions := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, groupID)

		if !hasPermissions {
			SetToast(c, "notuser")
			return c.Redirect(302, "/battle/submit")
		}
	}

	battleType := policy.Sanitize(c.FormValue("type"))

	if battleType != "beat" && battleType != "rap" {
		SetToast(c, "invalidtype")
		return c.Redirect(302, "/battle/submit")
	}

	if entries >= 3 {
		SetToast(c, "maxbattles")
		return c.Redirect(302, "/")
	}

	loc, err := time.LoadLocation(policy.Sanitize(c.FormValue("timezone")))
	if err != nil {
		loc, _ = time.LoadLocation("America/Toronto")
	}
	// For time.Parse
	layout := "Jan 2, 2006 03:04 PM"

	unparsedDeadline := policy.Sanitize(c.FormValue("deadline-date") + " " + c.FormValue("deadline-time"))
	deadline, err := time.ParseInLocation(layout, unparsedDeadline, loc)
	if err != nil || deadline.Before(time.Now()) {
		SetToast(c, "deadb4")
		return c.Redirect(302, "/battle/submit")
	}

	unparsedVotingDeadline := policy.Sanitize(c.FormValue("votingdeadline-date") + " " + c.FormValue("votingdeadline-time"))
	votingDeadline, err := time.ParseInLocation(layout, unparsedVotingDeadline, loc)
	if err != nil || votingDeadline.Before(deadline) {
		SetToast(c, "voteb4")
		return c.Redirect(302, "/battle/submit")
	}

	maxVotes, err := strconv.Atoi(policy.Sanitize(c.FormValue("maxvotes")))
	if err != nil || maxVotes < 1 || maxVotes > 10 {
		SetToast(c, "maxvotesinvalid")
		return c.Redirect(302, "/battle/submit")
	}

	/*
		attachmentURL, err := url.Parse(policy.Sanitize(c.FormValue("attachment")))
		if err != nil {
			SetToast(c, "unapprovedurl")
			return c.Redirect(302, "/battle/submit")
		}

		attachment := ""
		// PERF - MIGHT IMPACT A LOT
			if attachmentURL.String() != "" {
				if !contains(whitelist, strings.TrimPrefix(attachmentURL.Host, "www.")) {
					SetToast(c, "unapprovedurl")
					return c.Redirect(302, "/battle/submit")
				}
			}
	*/

	attachment := policy.Sanitize(c.FormValue("attachment"))

	status := "entry"
	if c.FormValue("submit") == "DRAFT" {
		status = "draft"
	}

	battle := &Battle{
		Title:          strings.TrimSpace(policy.Sanitize(c.FormValue("title"))),
		Rules:          strings.TrimSpace(policy.Sanitize(c.FormValue("rules"))),
		Deadline:       deadline,
		VotingDeadline: votingDeadline,
		Attachment:     attachment,
		Host:           me,
		Status:         status,
		Password:       policy.Sanitize(c.FormValue("password")),
		Entries:        0,
		ID:             0,
		MaxVotes:       maxVotes,
		GroupID:        groupID,
		Type:           battleType,
	}

	v := validator.New()
	err = v.Struct(battle)

	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			fmt.Println(err.Namespace())
		}
		SetToast(c, "validationerror")
		return c.Redirect(302, "/battle/submit")
	}

	if RowExists("SELECT id FROM challenges WHERE user_id = ? AND title = ?", me.ID, battle.Title) {
		SetToast(c, "titleexists")
		return c.Redirect(302, "/battle/submit")
	}

	stmt := "INSERT INTO challenges(title, rules, deadline, attachment, status, password, user_id, voting_deadline, maxvotes, group_id, type) VALUES(?,?,?,?,?,?,?,?,?,?,?)"

	ins, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer ins.Close()

	var battleInsertedID int64 = 0
	res, err := ins.Exec(battle.Title, battle.Rules, battle.Deadline, battle.Attachment, battle.Status, battle.Password,
		battle.Host.ID, battle.VotingDeadline, battle.MaxVotes, battle.GroupID, battle.Type)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	battleInsertedID, _ = res.LastInsertId()
	TagsDB(false, c.FormValue("tags"), battleInsertedID)

	SetToast(c, "successadd")
	return c.Redirect(302, "/battle/"+strconv.FormatInt(battleInsertedID, 10))
}

// TagsDB adds / updates tags in the DB.
func TagsDB(update bool, tagsJSON string, battleID int64) {
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
func GetTags(battleID int) []Tag {
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
	if err = rows.Err(); err != nil {
		// handle the error here
	}
	if err = rows.Close(); err != nil {
		// but what should we do if there's an error?
		log.Println(err)
	}

	return Tags
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
		stmt := "DELETE FROM challenges WHERE user_id = ? AND id = ?"

		del, err := db.Prepare(stmt)
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
	SetToast(c, "notuser")
	return c.Redirect(302, "/")
}
