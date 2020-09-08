package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/markbates/goth/gothic"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

// User struct.
type User struct {
	ID            int    `gorm:"column:id"`
	Provider      string `gorm:"column:provider"`
	ProviderID    string `gorm:"column:provider_id"`
	Name          string `gorm:"column:nickname"`
	NameHTML      string
	Avatar        string
	RefreshToken  string
	AccessToken   string    `gorm:"column:access_token"`
	ExpiresAt     time.Time `gorm:"column:expiry"`
	Authenticated bool
	Patron        bool   `gorm:"column:patron"`
	Flair         string `gorm:"column:flair"`
}

// HashAndSalt returns a hashed password.
func HashAndSalt(pwd []byte) string {
	// Use GenerateFromPassword to hash & salt pwd.
	// MinCost is just an integer constant provided by the bcrypt
	// package along with DefaultCost & MaxCost.
	// The cost can be any value you want provided it isn't lower
	// than the MinCost (4)
	hash, err := bcrypt.GenerateFromPassword(pwd, bcrypt.MinCost)
	if err != nil {
		log.Println(err)
	}
	// GenerateFromPassword returns a byte slice so we need to
	// convert the bytes to a string and return it
	return string(hash)
}

// ComparePasswords hashes the plain password and compares it to the stored hash.
func ComparePasswords(hashedPwd string, plainPwd []byte) bool {
	// Convert the string hash to a slice in order to compare.
	byteHash := []byte(hashedPwd)
	err := bcrypt.CompareHashAndPassword(byteHash, plainPwd)
	if err != nil {
		log.Println(err)
		return false
	}
	return true
}

// GetUserDB retrieves user from the database using a UserID.
func GetUserDB(UserID int) User {
	query := "SELECT provider, provider_id, nickname, patron, flair FROM users WHERE id = ?"

	user := User{}
	user.ID = UserID

	err := dbRead.QueryRow(query, UserID).Scan(&user.Provider, &user.ProviderID, &user.Name, &user.Patron, &user.Flair)
	if err != nil {
		log.Println(err)
		return User{}
	}

	user.NameHTML = `<a class="battle-url" href="/user/` + strconv.Itoa(user.ID) + `">` + user.Name + `</a>`
	if user.Patron {
		user.NameHTML = user.NameHTML + `&nbsp;<span class="user-flair material-icons tooltipped" data-tooltip="Patron">local_fire_department</span>`
	}

	return user
}

// Callback does the main heavy lifting of the 2FA authentication.
// This code is kind of messy and should be refactored.
func Callback(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	// Get the session.
	sess, err := store.Get(c.Request(), "beatbattleapp")
	if err != nil {
		fmt.Println(fmt.Sprintf("Callback - Session get err: %s", err))
	}

	user := User{}
	handler := c.QueryParam("provider")

	if handler != "reddit" {
		// Non-reddit oAuth requests are handled by gothic.
		gothUser, err := gothic.CompleteUserAuth(c.Response(), c.Request())
		if err != nil {
			// Delete session.
			sess.Options.MaxAge = -1
			err = sess.Save(c.Request(), c.Response())
			if err != nil {
				fmt.Println(fmt.Sprintf("Session save error: %s", err))
			}

			SetToast(c, "cache")
			fmt.Println(fmt.Sprintf("Goth authentication failure: %s", err))

			return c.Redirect(302, "/login")
		}

		// Set account details.
		user.Provider = gothUser.Provider
		user.Name = gothUser.Name
		user.Avatar = gothUser.AvatarURL
		user.ProviderID = gothUser.UserID

		// Set oAuth 2.0 tokens.
		user.RefreshToken = gothUser.RefreshToken
		user.AccessToken = gothUser.AccessToken
		user.ExpiresAt = gothUser.ExpiresAt

		// Set user as authenticated.
		user.Authenticated = true
	}

	if handler == "reddit" {
		// Retrieve state & code from the oAuth url.
		state := c.QueryParam("state")
		code := c.QueryParam("code")

		// Get a reddit token.
		token, err := redditAuth.GetToken(state, code)
		if err != nil {
			sess.Options.MaxAge = -1
			err = sess.Save(c.Request(), c.Response())
			if err != nil {
				fmt.Println(fmt.Sprintf("Session save error: %s", err))
			}

			fmt.Println(fmt.Sprintf("Reddit auth failure: %s", err))
			SetToast(c, "cache")

			return c.Redirect(302, "/login")
		}

		// Access the reddit cleint.
		client := redditAuth.GetAuthClient(token)
		redditUser, err := client.GetMe()
		if err != nil {
			sess.Options.MaxAge = -1
			err = sess.Save(c.Request(), c.Response())
			if err != nil {
				fmt.Println(fmt.Sprintf("Session save error: %s", err))
			}

			fmt.Println(fmt.Sprintf("Reddit client failure: %s", err))
			SetToast(c, "cache")

			return c.Redirect(302, "/login")
		}

		// Set account details.
		user.Provider = "reddit"
		user.Name = redditUser.Name
		user.Avatar = ""
		user.ProviderID = redditUser.ID

		// Set oAuth 2.0 tokens.
		user.RefreshToken = token.RefreshToken
		user.AccessToken = token.AccessToken
		user.ExpiresAt = token.Expiry

		// Set user as authenticated.
		user.Authenticated = true
	}

	// Check if user exists.
	userID := 0
	err = dbRead.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", user.Provider, user.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		fmt.Println(fmt.Sprintf("Checking to see if user exists failed: %s", err))
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	accessTokenEncrypted := HashAndSalt([]byte(user.AccessToken))
	// If user doesn't exist, add to db.
	if userID == 0 {
		sql := `INSERT INTO 
				users(provider, provider_id, nickname, access_token, expiry, patron, flair) 
				VALUES
				(?,?,?,?,?,?,?)`

		stmt, err := dbWrite.Prepare(sql)
		if err != nil {
			fmt.Println(fmt.Sprintf("User insert SQL failure: %s", err))
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}
		defer stmt.Close()
		stmt.Exec(user.Provider, user.ProviderID, user.Name, accessTokenEncrypted, user.ExpiresAt, 0, "")
	} else {
		sql := `UPDATE
				users 
				SET 
				nickname = ?, access_token = ?, expiry = ? WHERE id = ?`

		stmt, err := dbWrite.Prepare(sql)
		if err != nil {
			fmt.Println(fmt.Sprintf("User update SQL failure: %s", err))
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}
		defer stmt.Close()
		stmt.Exec(user.Name, accessTokenEncrypted, user.ExpiresAt, userID)
	}

	// Select user ID. This seems awfully unnecessary. We shoudl be able to get this from the insert statement.
	// TODO - Clean this up.
	if userID == 0 {
		err = dbRead.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", user.Provider, user.ProviderID).Scan(&userID)
		if err != nil && err != sql.ErrNoRows {
			fmt.Println(fmt.Sprintf("(SQL) Selecting user ID failed: %s", err))
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
	}

	user.ID = userID
	sess.Values["user"] = user

	err = sess.Save(c.Request(), c.Response())
	if err != nil {
		fmt.Println(fmt.Sprintf("Session save error: %s", err))
	}

	return c.Redirect(302, "/")
}

// Login returns the login page.
func Login(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	toast := GetToast(c)

	m := map[string]interface{}{
		"Title": "Login",
		"Toast": toast,
	}

	return c.Render(302, "Login", m)
}

// Auth routes the login request to the proper handler.
func Auth(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Retrieve the handler from the GET request.
	handler := c.QueryParam("provider")
	if handler == "reddit" {
		return c.Redirect(302, redditAuth.GetAuthenticationURL())
	}
	gothic.BeginAuthHandler(c.Response(), c.Request())
	return c.NoContent(302)
}

// Logout deletes the local session.
func Logout(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	gothic.Logout(c.Response(), c.Request())

	sess, _ := store.Get(c.Request(), "beatbattleapp")
	sess.Options.MaxAge = -1
	sess.Save(c.Request(), c.Response())

	return c.Redirect(302, "/")
}

// GetUser retrieves user details from local storage.
// If validation is required, it checks if the access token is expired.
// REVIEW - not slow, but confusing
func GetUser(c echo.Context, validate bool) User {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	var user User
	user.ID = 0

	sess, err := store.Get(c.Request(), "beatbattleapp")
	if err != nil {
		fmt.Println(fmt.Sprintf("Session get err: %s", err))
	}

	if sess.Values["user"] != nil {
		user = sess.Values["user"].(User)

		// Kick out users who haven't logged in since the update.
		if user.AccessToken == "" {
			sess.Values["user"] = User{}
			sess.Save(c.Request(), c.Response())
			SetToast(c, "relog")
			return User{}
		}

		if validate {
			var dbHash string
			err := dbRead.QueryRow("SELECT access_token, expiry FROM users WHERE id = ?", user.ID).Scan(&dbHash, &user.ExpiresAt)
			if err != nil {
				fmt.Println(fmt.Sprintf("(SQL) Selecting access token & expiry failed: %s", err))
				return User{}
			}

			// Is access_token expired?
			if time.Until(user.ExpiresAt) < 0 {
				var newToken *oauth2.Token
				// Refresh Access Token
				if user.Provider == "discord" {
					newToken, err = discordProvider.RefreshToken(user.RefreshToken)
					if err != nil {
						fmt.Println(fmt.Sprintf("(AUTH) Requesting discord refresh token failed: %s", err))
						sess.Values["user"] = User{}
						sess.Save(c.Request(), c.Response())
						SetToast(c, "relog")
						return User{}
					}
				}

				// TODO - Refresh reddit token!
				if user.Provider == "reddit" {
					return user
				}

				user.AccessToken = newToken.AccessToken
				user.RefreshToken = newToken.RefreshToken
				user.ExpiresAt = newToken.Expiry
				user.Authenticated = true

				sql := "UPDATE users SET access_token = ?, expiry = ? WHERE id = ?"

				// If we can't update the users in the database, destroy the session.
				stmt, err := dbWrite.Prepare(sql)
				if err != nil {
					fmt.Println(fmt.Sprintf("(SQL) Cant update DB user, destroying session: %s", err))
					sess.Values["user"] = User{}
					sess.Save(c.Request(), c.Response())
					SetToast(c, "cache")
					return User{}
				}
				defer stmt.Close()

				accessTokenEncrypted := HashAndSalt([]byte(user.AccessToken))
				dbHash = accessTokenEncrypted
				stmt.Exec(accessTokenEncrypted, user.ExpiresAt, user.ID)
			}

			if !ComparePasswords(dbHash, []byte(user.AccessToken)) {
				fmt.Println(fmt.Sprintf("(SQL) Hashed access token mismatch: %s", err))
				sess.Values["user"] = User{}
				sess.Save(c.Request(), c.Response())
				SetToast(c, "relog")
				return User{}
			}
		}

		sess.Values["user"] = user
		sess.Save(c.Request(), c.Response())
	}

	return user
}

// AjaxResponse ...
func AjaxResponse(c echo.Context, redirect bool, redirectPath string, toastQuery string) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	type AjaxData struct {
		Redirect     bool   `json:"Redirect"`
		RedirectPath string `json:"RedirectPath"`
		ToastHTML    string `json:"ToastHTML"`
		ToastClass   string `json:"ToastClass"`
		ToastQuery   string `json:"ToastQuery"`
	}

	SetToast(c, toastQuery)
	toast := GetToast(c)

	data := new(AjaxData)

	data.Redirect = redirect
	data.RedirectPath = redirectPath
	data.ToastHTML = toast[0]
	data.ToastClass = toast[1]
	data.ToastQuery = toastQuery

	if err := c.Bind(data); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, data)
}

// AddVote is a user function that grabs the logged in user object and adds a vote to the DB.
func AddVote(c echo.Context) error {
	start := time.Now()

	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	// Get user, return if not auth.
	me := GetUser(c, true)
	if !me.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	// Get form values.
	beatID, err := strconv.Atoi(c.FormValue("beatID"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	battleID, err := strconv.Atoi(c.FormValue("battleID"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	beatUserID, err := strconv.Atoi(c.FormValue("userID"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	// Reject if user ID matches the track.
	if beatUserID == me.ID {
		return AjaxResponse(c, false, redirectURL, "owntrack")
	}

	// Get battle status, max votes, and vote array.
	status := ""
	maxVotes := 1
	var voteArray []uint8
	err = dbRead.QueryRow(
		`SELECT battle.status, battle.maxvotes, GROUP_CONCAT(DISTINCT IFNULL(votes.beat_id, '')) AS user_votes
		FROM (SELECT status, maxvotes, id FROM challenges WHERE challenges.id = ?) battle
		LEFT JOIN (SELECT beat_id, challenge_id FROM votes WHERE user_id = ? AND challenge_id = ? ORDER BY beat_id) votes
		ON battle.id = votes.challenge_id`,
		// Fill in
		battleID, me.ID, battleID).Scan(
		//
		&status, &maxVotes, &voteArray)
	if err != nil && err != sql.ErrNoRows {
		return AjaxResponse(c, true, "/", "502")
	}

	// Reject if not currently in voting stage or if challenge is invalid.
	if err == sql.ErrNoRows || status != "voting" {
		return AjaxResponse(c, true, redirectURL, "302")
	}

	log.Println(voteArray)
	voteString := string(voteArray)
	voteStringArray := strings.Split(voteString, ",")
	var userVotes []int
	for _, s := range voteStringArray {
		voteID, _ := strconv.Atoi(s)
		if voteID != 0 {
			userVotes = append(userVotes, voteID)
		}
	}

	if len(userVotes) < maxVotes {
		// If a vote for this beat does not exist
		if !ContainsInt(userVotes, beatID) {
			// Add a vote to the vote table for the beat.
			ins, err := dbWrite.Prepare("INSERT INTO votes(beat_id, user_id, challenge_id) VALUES(?,?,?)")
			if err != nil {
				return AjaxResponse(c, true, redirectURL, "404")
			}
			defer ins.Close()
			ins.Exec(beatID, me.ID, battleID)

			duration := time.Since(start)
			fmt.Println("AddVote time: " + duration.String())

			return AjaxResponse(c, false, redirectURL, "successvote")
		} else if ContainsInt(userVotes, beatID) {
			// Delete vote from the votes table.
			del, err := dbWrite.Prepare("DELETE FROM votes WHERE beat_id = ? AND user_id = ? AND challenge_id = ?")
			if err != nil {
				return AjaxResponse(c, true, redirectURL, "404")
			}
			defer del.Close()
			del.Exec(beatID, me.ID, battleID)

			duration := time.Since(start)
			fmt.Println("AddVote time: " + duration.String())
			return AjaxResponse(c, false, redirectURL, "successdelvote")
		}
	} else {
		// If a vote doesn't exist, return the user.
		if !ContainsInt(userVotes, beatID) {
			return AjaxResponse(c, false, redirectURL, "maxvotes")
		}

		// Delete vote from the votes table.
		del, err := dbWrite.Prepare("DELETE FROM votes WHERE beat_id = ? AND user_id = ? AND challenge_id = ?")
		if err != nil {
			return AjaxResponse(c, true, redirectURL, "404")
		}
		defer del.Close()
		del.Exec(beatID, me.ID, battleID)

		duration := time.Since(start)
		fmt.Println("AddVote time: " + duration.String())
		return AjaxResponse(c, false, redirectURL, "successdelvote")
	}

	duration := time.Since(start)
	fmt.Println("AddVote time: " + duration.String())

	return AjaxResponse(c, false, redirectURL, "404")
}

// AddLike ...
func AddLike(c echo.Context) error {
	start := time.Now()
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)
	if !me.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	beatID, err := strconv.Atoi(c.FormValue("beatID"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	battleID, err := strconv.Atoi(c.FormValue("battleID"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if !RowExists("SELECT user_id FROM likes WHERE user_id = ? AND beat_id = ?", me.ID, beatID) {
		ins, err := dbWrite.Prepare("INSERT INTO likes(user_id, beat_id, challenge_id) VALUES (?, ?, ?)")
		if err != nil {
			return AjaxResponse(c, true, "/", "502")
		}
		defer ins.Close()
		ins.Exec(me.ID, beatID, battleID)
		duration := time.Since(start)
		fmt.Println("AddLike time: " + duration.String())
		return AjaxResponse(c, false, redirectURL, "liked")
	}

	del, err := dbWrite.Prepare("DELETE from likes WHERE user_id = ? AND beat_id = ? AND challenge_id = ?")
	if err != nil {
		return AjaxResponse(c, true, "/", "502")
	}
	defer del.Close()
	del.Exec(me.ID, beatID, battleID)

	duration := time.Since(start)
	fmt.Println("AddLike time: " + duration.String())

	return AjaxResponse(c, false, redirectURL, "unliked")
}

// AddFeedback ...
func AddFeedback(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)
	if !me.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	beatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	var battleID int
	var userID int
	feedback := policy.Sanitize(c.FormValue("feedback"))

	err = dbRead.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &userID)
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if userID == me.ID {
		return AjaxResponse(c, false, "/", "feedbackself")
	}

	if !RowExists("SELECT id FROM feedback WHERE user_id = ? AND beat_id = ?", me.ID, beatID) {
		ins, err := dbWrite.Prepare("INSERT INTO feedback(feedback, user_id, beat_id) VALUES (?, ?, ?)")
		if err != nil {
			return AjaxResponse(c, true, "/", "502")
		}
		defer ins.Close()
		ins.Exec(feedback, me.ID, beatID)
		return AjaxResponse(c, false, redirectURL, "successaddfeedback")
	}

	update, err := dbWrite.Prepare("UPDATE feedback SET feedback = ? WHERE user_id = ? AND beat_id = ?")
	if err != nil {
		return AjaxResponse(c, true, "/", "502")
	}
	defer update.Close()
	update.Exec(feedback, me.ID, beatID)

	return AjaxResponse(c, false, redirectURL, "successupdate")
}

// ViewFeedback - Retrieves user's feedback and returns a page containing them.
func ViewFeedback(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if the user is properly authenticated.
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	ads := GetAdvertisements()
	toast := GetToast(c)
	battleID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// Retrieve battle, return to front page if battle doesn't exist.
	battle := GetBattle(battleID)
	if battle.Title == "" {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	query := `SELECT users.nickname, feedback.feedback
				FROM beats
				LEFT JOIN feedback on feedback.beat_id = beats.id
				LEFT JOIN users on feedback.user_id = users.id
				WHERE beats.challenge_id = ? AND beats.user_id = ? AND feedback.feedback IS NOT NULL`

	rows, err := dbRead.Query(query, battleID, me.ID)
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	type Feedback struct {
		From     string `json:"from"`
		Feedback string `json:"feedback"`
	}

	curFeedback := Feedback{}
	feedback := []Feedback{}

	for rows.Next() {
		err = rows.Scan(&curFeedback.From, &curFeedback.Feedback)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		feedback = append(feedback, curFeedback)
	}
	// Reference: http://go-database-sql.org/errors.html - I'm not really sure if this does anything positive lmao.
	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	feedbackJSON, err := json.Marshal(feedback)

	m := map[string]interface{}{
		"Title":    battle.Title,
		"Buttons":  "Feedback",
		"Battle":   battle,
		"Feedback": string(feedbackJSON),
		"Me":       me,
		"Toast":    toast,
		"Ads":      ads,
		// Fix this hardwrite
		"EnteredBattle": "true",
	}

	return c.Render(302, "Feedback", m)
}

// UserBattles - Retrieves user's battles and returns a page containing them.
func UserBattles(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated and retrieve any groups that they have invite privileges to.
	// This is for the invite functionality.
	userID := 0
	user := User{}
	me := GetUser(c, false)
	userGroups := []Group{}
	if me.Authenticated {
		userGroups = GetGroupsByRole(dbRead, me.ID, "owner")
	}

	toast := GetToast(c)
	ads := GetAdvertisements()
	title := ""

	// Is this a request to check their own account?
	if c.Request().URL.String() == "/me" {
		userID = me.ID
		user = GetUserDB(userID)
		title = "My"
	} else {
		userID, _ = strconv.Atoi(c.Param("id"))
		user = GetUserDB(userID)
		title = user.Name + "'s"
	}

	battles := GetBattles("challenges.user_id", strconv.Itoa(userID))
	battlesJSON, _ := json.Marshal(battles)

	m := map[string]interface{}{
		"Title":      title + " Battles",
		"Page":       "battles",
		"Battles":    string(battlesJSON),
		"Me":         me,
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Tag":        policy.Sanitize(c.Param("tag")),
		"Ads":        ads,
	}

	return c.Render(302, "UserBattles", m)
}

// UserSubmissions - Retrieves user's submissions and returns a page containing them.
func UserSubmissions(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated and retrieve any groups that they have invite privileges to.
	// This is for the invite functionality.
	userID := 0
	user := User{}
	me := GetUser(c, false)
	userGroups := []Group{}
	if me.Authenticated {
		userGroups = GetGroupsByRole(dbRead, me.ID, "owner")
	}

	toast := GetToast(c)
	ads := GetAdvertisements()
	title := ""

	// Is this a request to check their own account?
	if c.Request().URL.String() == "/me/submissions" {
		userID = me.ID
		user = GetUserDB(userID)
		title = "My"
	} else {
		userID, _ = strconv.Atoi(c.Param("id"))
		user = GetUserDB(userID)
		title = user.Name + "'s"
	}

	submission := Beat{}
	entries := []Beat{}

	query := `
			SELECT beats.url, beats.votes, beats.voted, challenges.id, challenges.status, challenges.title
			FROM beats
			LEFT JOIN challenges on challenges.id=beats.challenge_id
			WHERE beats.user_id=?
			GROUP BY 1
			ORDER BY beats.id DESC`

	rows, err := dbRead.Query(query, userID)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	for rows.Next() {
		submission = Beat{}
		err = rows.Scan(&submission.URL, &submission.Votes, &submission.Voted, &submission.ChallengeID, &submission.Status, &submission.Battle)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		submission.Status = strings.Title(submission.Status)
		if !submission.Voted && submission.Status == "complete" {
			submission.Status = `<span class="tooltipped" data-tooltip="Did Not Vote">` + submission.Status + ` <span style="color: #0D88FF;">(*)</span></span>`
		}

		u, _ := url.Parse(submission.URL)
		urlSplit := strings.Split(u.RequestURI(), "/")

		if len(urlSplit) >= 4 {
			secretURL := urlSplit[3]
			if strings.Contains(secretURL, "s-") {
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' show_user='false' src='https://w.soundcloud.com/player/?url=https://soundcloud.com/` + urlSplit[1] + "/" + urlSplit[2] + `?secret_token=` + urlSplit[3] + `&color=%23ff5500&inverse=false&autoplay=true&show_user=false'></iframe>`
			} else {
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=true&show_user=false'></iframe>`
			}
		} else {
			submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=true&show_user=false'></iframe>`
		}

		entries = append(entries, submission)
	}
	// Reference: http://go-database-sql.org/errors.html - I'm not really sure if this does anything positive lmao.
	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	submissionsJSON, err := json.Marshal(entries)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	m := map[string]interface{}{
		"Title":      title + " Submissions",
		"Page":       "submissions",
		"Beats":      string(submissionsJSON),
		"Me":         me,
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Tag":        policy.Sanitize(c.Param("tag")),
		"Ads":        ads,
	}

	return c.Render(302, "UserSubmissions", m)
}

// UserTrophies - Retrieves user's victories and returns a page containing them.
func UserTrophies(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated and retrieve any groups that they have invite privileges to.
	// This is for the invite functionality.
	userID := 0
	user := User{}
	me := GetUser(c, false)
	userGroups := []Group{}
	if me.Authenticated {
		userGroups = GetGroupsByRole(dbRead, me.ID, "owner")
	}

	toast := GetToast(c)
	ads := GetAdvertisements()
	title := ""

	// Is this a request to check their own account?
	if c.Request().URL.String() == "/me/submissions" {
		userID = me.ID
		user = GetUserDB(userID)
		title = "My"
	} else {
		userID, _ = strconv.Atoi(c.Param("id"))
		user = GetUserDB(userID)
		title = user.Name + "'s"
	}

	submission := Beat{}
	entries := []Beat{}

	query := `
			SELECT beats.url, beats.votes, beats.voted, challenges.id, challenges.status, challenges.title
			FROM beats
			LEFT JOIN challenges on challenges.id=beats.challenge_id
			WHERE beats.user_id=?
			GROUP BY 1
			ORDER BY beats.id DESC`

	rows, err := dbRead.Query(query, userID)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	for rows.Next() {
		submission = Beat{}
		err = rows.Scan(&submission.URL, &submission.Votes, &submission.Voted, &submission.ChallengeID, &submission.Status, &submission.Battle)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		submission.Status = strings.Title(submission.Status)
		if !submission.Voted {
			submission.Status = `<span class="tooltipped" data-tooltip="Did Not Vote">` + submission.Status + ` <span style="color: #0D88FF;">(*)</span></span>`
		}

		u, _ := url.Parse(submission.URL)
		urlSplit := strings.Split(u.RequestURI(), "/")

		if len(urlSplit) >= 4 {
			secretURL := urlSplit[3]
			if strings.Contains(secretURL, "s-") {
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' show_user='false' src='https://w.soundcloud.com/player/?url=https://soundcloud.com/` + urlSplit[1] + "/" + urlSplit[2] + `?secret_token=` + urlSplit[3] + `&color=%23ff5500&inverse=false&autoplay=true&show_user=false'></iframe>`
			} else {
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=true&show_user=false'></iframe>`
			}
		} else {
			submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=true&show_user=false'></iframe>`
		}

		entries = append(entries, submission)
	}
	// Reference: http://go-database-sql.org/errors.html - I'm not really sure if this does anything positive lmao.
	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	submissionsJSON, err := json.Marshal(entries)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	m := map[string]interface{}{
		"Title":      title + " Submissions",
		"Page":       "submissions",
		"Beats":      string(submissionsJSON),
		"Me":         me,
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Tag":        policy.Sanitize(c.Param("tag")),
		"Ads":        ads,
	}

	return c.Render(302, "UserSubmissions", m)
}

// UserGroups - Retrieves user's groups and returns a page containing them.
func UserGroups(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated and retrieve any groups that they have invite privileges to.
	// This is for the invite functionality.
	user := User{}
	me := GetUser(c, false)
	userGroups := []Group{}
	if me.Authenticated {
		userGroups = GetGroupsByRole(dbRead, me.ID, "owner")
	}

	toast := GetToast(c)
	ads := GetAdvertisements()
	title := ""

	requestsString, invitesString, groupsString := "", "", ""

	// Is this a request to check their own account?
	if c.Request().URL.String() == "/me/groups" {
		user = GetUserDB(me.ID)
		title = "My"

		requests, invites, groups := GetUserGroups(dbRead, user.ID)

		requestsJSON, _ := json.Marshal(requests)
		invitesJSON, _ := json.Marshal(invites)
		groupsJSON, _ := json.Marshal(groups)

		requestsString = string(requestsJSON)
		invitesString = string(invitesJSON)
		groupsString = string(groupsJSON)

		if requestsString == "[]" {
			requestsString = ""
		}
		if invitesString == "[]" {
			invitesString = ""
		}
		if groupsString == "[]" {
			groupsString = ""
		}
	} else {
		userID, _ := strconv.Atoi(c.Param("id"))
		user = GetUserDB(userID)
		title = user.Name + "'s"

		groups := GetGroups(dbRead, user.ID)
		groupsJSON, _ := json.Marshal(groups)
		groupsString = string(groupsJSON)
	}

	m := map[string]interface{}{
		"Title":      title + " Groups",
		"Page":       "groups",
		"Requests":   requestsString,
		"Invites":    invitesString,
		"Groups":     groupsString,
		"Me":         me,
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Ads":        ads,
	}

	return c.Render(302, "UserGroups", m)
}
