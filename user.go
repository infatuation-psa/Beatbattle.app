package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo-contrib/session"
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
	rows, err := db.Query(query, UserID)
	if err != nil {
		log.Println(err)
		return User{}
	}
	defer rows.Close()

	user := User{}
	user.ID = UserID

	for rows.Next() {
		err = rows.Scan(&user.Provider, &user.ProviderID, &user.Name, &user.Patron, &user.Flair)
		if err != nil {
			log.Println(err)
			return User{}
		}
	}

	user.NameHTML = user.Name
	if user.Patron {
		user.NameHTML = user.NameHTML + `&nbsp;<span class="material-icons tooltipped" data-tooltip="Patron">local_fire_department</span>`
	}

	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	return user
}

// Callback does the main heavy lifting of the 2FA authentication.
// This code is kind of messy and should be refactored.
func Callback(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	sess, _ := session.Get("beatbattle", c)
	Account := User{}
	handler := c.QueryParam("provider")

	if handler != "reddit" {
		user, err := gothic.CompleteUserAuth(c.Response(), c.Request())

		if err != nil {
			sess.Options.MaxAge = -1
			sess.Save(c.Request(), c.Response())
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}

		Account.Provider = user.Provider
		Account.Name = user.Name
		Account.Avatar = user.AvatarURL
		Account.ProviderID = user.UserID

		// Auth
		Account.RefreshToken = user.RefreshToken
		Account.AccessToken = user.AccessToken
		Account.ExpiresAt = user.ExpiresAt
		Account.Authenticated = true
	}

	if handler == "reddit" {
		state := c.QueryParam("state")
		code := c.QueryParam("code")
		token, err := redditAuth.GetToken(state, code)
		if err != nil {
			sess.Options.MaxAge = -1
			sess.Save(c.Request(), c.Response())
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}
		client := redditAuth.GetAuthClient(token)
		user, err := client.GetMe()
		if err != nil {
			sess.Options.MaxAge = -1
			sess.Save(c.Request(), c.Response())
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}
		Account.Provider = "reddit"
		Account.Name = user.Name
		Account.Avatar = ""
		Account.ProviderID = user.ID

		// Auth
		Account.RefreshToken = token.RefreshToken
		Account.AccessToken = token.AccessToken
		Account.ExpiresAt = token.Expiry
		Account.Authenticated = true
	}

	userID := 0
	err := db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	accessTokenEncrypted := HashAndSalt([]byte(Account.AccessToken))
	// If user doesn't exist, add to db
	if userID == 0 {
		sql := "INSERT INTO users(provider, provider_id, nickname, access_token, expiry, patron, flair) VALUES(?,?,?,?,?,?,?)"

		stmt, err := db.Prepare(sql)
		if err != nil {
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}
		defer stmt.Close()

		stmt.Exec(Account.Provider, Account.ProviderID, Account.Name, accessTokenEncrypted, Account.ExpiresAt, 0, 0)
	} else {
		sql := "UPDATE users SET nickname = ?, access_token = ?, expiry = ? WHERE id = ?"

		stmt, err := db.Prepare(sql)
		if err != nil {
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}
		defer stmt.Close()

		stmt.Exec(Account.Name, accessTokenEncrypted, Account.ExpiresAt, userID)
	}

	err = db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	Account.ID = userID
	sess.Values["user"] = Account

	sess.Save(c.Request(), c.Response())
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

	sess, _ := session.Get("beatbattle", c)
	sess.Options.MaxAge = -1
	sess.Save(c.Request(), c.Response())

	return c.Redirect(302, "/")
}

// GetUser retrieves user details from local storage.
// If validation is required, it checks if the access token is expired.
func GetUser(c echo.Context, validate bool) User {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	var user User
	user.ID = 0

	sess, _ := session.Get("beatbattle", c)
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
			err := db.QueryRow("SELECT access_token, expiry FROM users WHERE id = ?", user.ID).Scan(&dbHash, &user.ExpiresAt)
			if err != nil {
				return User{}
			}

			// Is access_token expired?
			if time.Until(user.ExpiresAt) < 0 {
				var newToken *oauth2.Token
				// Refresh Access Token
				if user.Provider == "discord" {
					newToken, err = discordProvider.RefreshToken(user.RefreshToken)
					if err != nil {
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
				stmt, err := db.Prepare(sql)
				if err != nil {
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
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)
	if !me.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	beatID, err := strconv.Atoi(c.FormValue("id"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	var battleID int
	var beatUserID int

	// Get battle (challenge) ID and user ID from the beat ID.
	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &beatUserID)
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	// Get battle status & max votes.
	status := ""
	maxVotes := 1
	err = db.QueryRow("SELECT status, maxvotes FROM challenges WHERE id = ?", battleID).Scan(&status, &maxVotes)
	if err != nil && err != sql.ErrNoRows {
		return AjaxResponse(c, true, "/", "502")
	}

	// Reject if not currently in voting stage or if challenge is invalid.
	if err == sql.ErrNoRows || status != "voting" {
		return AjaxResponse(c, true, redirectURL, "302")
	}

	// Reject if user ID matches the track.
	if beatUserID == me.ID {
		return AjaxResponse(c, false, redirectURL, "owntrack")
	}

	count := 0
	_ = db.QueryRow("SELECT COUNT(id) FROM votes WHERE user_id = ? AND challenge_id = ?", me.ID, battleID).Scan(&count)

	voteID := 0
	voteErr := db.QueryRow("SELECT id FROM votes WHERE user_id = ? AND beat_id = ?", me.ID, beatID).Scan(&voteID)

	// TODO Change from transaction maybe

	if count < maxVotes {
		// If a vote for this beat does not exist
		if voteErr == sql.ErrNoRows {
			tx, err := db.Begin()
			if err != nil {
				return AjaxResponse(c, true, redirectURL, "404")
			}

			// Add a vote to the vote table for the beat.
			sql := "INSERT INTO votes(beat_id, user_id, challenge_id) VALUES(?,?,?)"
			ins, err := tx.Prepare(sql)
			if err != nil {
				return AjaxResponse(c, true, redirectURL, "404")
			}
			defer ins.Close()
			ins.Exec(beatID, me.ID, battleID)

			// Mark user as having voted if they've entered the battle themselves.
			votedSQL := "UPDATE beats SET voted = 1 WHERE user_id = ? AND challenge_id = ?"
			voted, _ := tx.Prepare(votedSQL)
			defer voted.Close()
			voted.Exec(me.ID, battleID)

			// Update the hard written votes on the beat.
			updSQL := "UPDATE beats SET votes = votes + 1 WHERE id = ?"
			upd, err := tx.Prepare(updSQL)
			if err != nil {
				return AjaxResponse(c, true, redirectURL, "404")
			}
			defer upd.Close()
			upd.Exec(beatID)

			// Commit the changes.
			tx.Commit()
			return AjaxResponse(c, false, redirectURL, "successvote")
		} else if voteErr == nil {
			// Delete vote from the votes table.
			tx, err := db.Begin()
			sql := "DELETE FROM votes WHERE id = ?"
			stmt, err := tx.Prepare(sql)
			if err != nil {
				return AjaxResponse(c, true, redirectURL, "404")
			}
			defer stmt.Close()
			stmt.Exec(voteID)

			// Remove vote from the beats table.
			updSQL := "UPDATE beats SET votes = votes - 1 WHERE id = ?"
			upd, err := tx.Prepare(updSQL)
			if err != nil {
				return AjaxResponse(c, true, redirectURL, "404")
			}
			defer upd.Close()
			upd.Exec(beatID)

			if count-1 == 0 {
				// Mark user as having voted if they've entered the battle themselves.
				votedSQL := "UPDATE beats SET voted = 0 WHERE user_id = ? AND challenge_id = ?"
				voted, _ := tx.Prepare(votedSQL)
				defer voted.Close()
				voted.Exec(me.ID, battleID)
			}

			// Commit and return deleted vote.
			tx.Commit()
			return AjaxResponse(c, false, redirectURL, "successdelvote")
		}
	} else {
		// If a vote doesn't exist, return the user.
		if voteErr == sql.ErrNoRows {
			return AjaxResponse(c, false, redirectURL, "maxvotes")
		}
		// Delete vote from the votes table.
		tx, err := db.Begin()
		sql := "DELETE FROM votes WHERE id = ?"
		stmt, err := tx.Prepare(sql)
		if err != nil {
			return AjaxResponse(c, true, redirectURL, "404")
		}
		defer stmt.Close()
		stmt.Exec(voteID)

		// Remove vote from the beats table.
		updSQL := "UPDATE beats SET votes = votes - 1 WHERE id = ?"
		upd, err := tx.Prepare(updSQL)
		if err != nil {
			return AjaxResponse(c, true, redirectURL, "404")
		}
		defer upd.Close()
		upd.Exec(beatID)

		if count-1 == 0 {
			// Mark user as having voted if they've entered the battle themselves.
			votedSQL := "UPDATE beats SET voted = 0 WHERE user_id = ? AND challenge_id = ?"
			voted, _ := tx.Prepare(votedSQL)
			defer voted.Close()
			voted.Exec(me.ID, battleID)
		}

		// Commit and return deleted vote.
		tx.Commit()
		return AjaxResponse(c, false, redirectURL, "successdelvote")
	}

	return AjaxResponse(c, false, redirectURL, "404")
}

// AddLike ...
func AddLike(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)
	if !me.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	beatID, err := strconv.Atoi(c.FormValue("id"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	var battleID int
	var userID int

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &userID)
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if !RowExists("SELECT user_id FROM likes WHERE user_id = ? AND beat_id = ?", me.ID, beatID) {
		ins, err := db.Prepare("INSERT INTO likes(user_id, beat_id, challenge_id) VALUES (?, ?, ?)")
		if err != nil {
			return AjaxResponse(c, true, "/", "502")
		}
		defer ins.Close()
		ins.Exec(me.ID, beatID, battleID)
		return AjaxResponse(c, false, redirectURL, "liked")
	}

	del, err := db.Prepare("DELETE from likes WHERE user_id = ? AND beat_id = ? AND challenge_id = ?")
	if err != nil {
		return AjaxResponse(c, true, "/", "502")
	}
	defer del.Close()
	del.Exec(me.ID, beatID, battleID)

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

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &userID)
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if userID == me.ID {
		return AjaxResponse(c, false, "/", "feedbackself")
	}

	if !RowExists("SELECT id FROM feedback WHERE user_id = ? AND beat_id = ?", me.ID, beatID) {
		ins, err := db.Prepare("INSERT INTO feedback(feedback, user_id, beat_id) VALUES (?, ?, ?)")
		if err != nil {
			return AjaxResponse(c, true, "/", "502")
		}
		defer ins.Close()
		ins.Exec(feedback, me.ID, beatID)
		return AjaxResponse(c, false, redirectURL, "successaddfeedback")
	}

	update, err := db.Prepare("UPDATE feedback SET feedback = ? WHERE user_id = ? AND beat_id = ?")
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

	rows, err := db.Query(query, battleID, me.ID)
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
		"Battle":   battle,
		"Feedback": string(feedbackJSON),
		"Me":       me,
		"Toast":    toast,
		"Ads":      ads,
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
		userGroups = GetGroupsByRole(db, me.ID, "owner")
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
		userGroups = GetGroupsByRole(db, me.ID, "owner")
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

	rows, err := db.Query(query, userID)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	ua := c.Request().Header.Get("User-Agent")
	mobileUA := regexp.MustCompile(`/Mobile|Android|BlackBerry|iPhone/`)
	isMobile := mobileUA.MatchString(ua)

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
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' show_user='false' src='https://w.soundcloud.com/player/?url=https://soundcloud.com/` + urlSplit[1] + "/" + urlSplit[2] + `?secret_token=` + urlSplit[3] + `&color=%23ff5500&inverse=false&autoplay=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
			} else {
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
			}
		} else {
			submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
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
		"IsMobile":   isMobile,
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
		userGroups = GetGroupsByRole(db, me.ID, "owner")
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

	rows, err := db.Query(query, userID)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	ua := c.Request().Header.Get("User-Agent")
	mobileUA := regexp.MustCompile(`/Mobile|Android|BlackBerry|iPhone/`)
	isMobile := mobileUA.MatchString(ua)

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
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' show_user='false' src='https://w.soundcloud.com/player/?url=https://soundcloud.com/` + urlSplit[1] + "/" + urlSplit[2] + `?secret_token=` + urlSplit[3] + `&color=%23ff5500&inverse=false&autoplay=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
			} else {
				submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
			}
		} else {
			submission.URL = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=` + submission.URL + `&color=%23ff5500&inverse=false&autoplay=` + strconv.FormatBool(!isMobile) + `&show_user=false'></iframe>`
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
		"IsMobile":   isMobile,
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
		userGroups = GetGroupsByRole(db, me.ID, "owner")
	}

	toast := GetToast(c)
	ads := GetAdvertisements()
	title := ""

	requestsString, invitesString, groupsString := "", "", ""

	// Is this a request to check their own account?
	if c.Request().URL.String() == "/me/groups" {
		user = GetUserDB(me.ID)
		title = "My"

		requests, invites, groups := GetUserGroups(db, user.ID)

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

		groups := GetGroups(db, user.ID)
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
