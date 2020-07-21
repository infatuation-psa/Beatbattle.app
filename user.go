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
	Avatar        string
	RefreshToken  string
	AccessToken   string    `gorm:"column:access_token"`
	ExpiresAt     time.Time `gorm:"column:expiry"`
	Authenticated bool
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

func comparePasswords(hashedPwd string, plainPwd []byte) bool {
	// Since we'll be getting the hashed password from the DB it
	// will be a string so we'll need to convert it to a byte slice
	byteHash := []byte(hashedPwd)
	err := bcrypt.CompareHashAndPassword(byteHash, plainPwd)
	if err != nil {
		log.Println(err)
		return false
	}

	return true
}

// Callback ...
func Callback(c echo.Context) error {
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
		sql := "INSERT INTO users(provider, provider_id, nickname, access_token, expiry) VALUES(?,?,?,?,?)"

		stmt, err := db.Prepare(sql)
		if err != nil {
			SetToast(c, "cache")
			return c.Redirect(302, "/login")
		}
		defer stmt.Close()

		stmt.Exec(Account.Provider, Account.ProviderID, Account.Name, accessTokenEncrypted, Account.ExpiresAt)
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

// Login ...
func Login(c echo.Context) error {
	toast := GetToast(c)

	m := map[string]interface{}{
		"Title": "Login",
		"Toast": toast,
	}

	return c.Render(302, "Login", m)
}

// Auth ...
func Auth(c echo.Context) error {
	handler := c.QueryParam("provider")

	if handler == "reddit" {
		return c.Redirect(302, redditAuth.GetAuthenticationURL())
	}

	gothic.BeginAuthHandler(c.Response(), c.Request())

	return c.NoContent(302)
}

// Logout ...
func Logout(c echo.Context) error {
	gothic.Logout(c.Response(), c.Request())

	sess, _ := session.Get("beatbattle", c)
	sess.Options.MaxAge = -1
	sess.Save(c.Request(), c.Response())

	return c.Redirect(302, "/")
}

// GetUser ...
func GetUser(c echo.Context, validate bool) User {
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

				if user.Provider == "reddit" {
					// TODO - Refresh reddit token
					return user
				}

				user.AccessToken = newToken.AccessToken
				user.RefreshToken = newToken.RefreshToken
				user.ExpiresAt = newToken.Expiry
				user.Authenticated = true

				sql := "UPDATE users SET access_token = ?, expiry = ? WHERE id = ?"

				stmt, err := db.Prepare(sql)
				if err != nil {
					SetToast(c, "cache")
					return User{}
				}
				defer stmt.Close()

				accessTokenEncrypted := HashAndSalt([]byte(user.AccessToken))
				dbHash = accessTokenEncrypted
				stmt.Exec(accessTokenEncrypted, user.ExpiresAt, user.ID)
			}

			if !comparePasswords(dbHash, []byte(user.AccessToken)) {
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

// CalculateVoted - Manual function to force vote recalculation.
func CalculateVoted(c echo.Context) error {
	var user = GetUser(c, true)

	if user.ID != 3 {
		SetToast(c, "notauth")
		return c.Redirect(302, "/")
	}

	query := `SELECT votes.user_id, votes.challenge_id, beats.id FROM votes 
				LEFT JOIN beats on beats.challenge_id=votes.challenge_id AND votes.user_id=beats.user_id`
	rows, err := db.Query(query)

	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	for rows.Next() {
		userID := 0
		challengeID := 0
		beatID := 0
		rows.Scan(&userID, &challengeID, &beatID)

		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		updateQuery := "UPDATE beats SET voted = 1 WHERE user_id = ? AND id = ?"

		upd, err := db.Prepare(updateQuery)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		defer upd.Close()

		upd.Exec(userID, beatID)
	}
	if err = rows.Err(); err != nil {
		// handle the error here
	}
	if err = rows.Close(); err != nil {
		// but what should we do if there's an error?
		log.Println(err)
	}

	return c.NoContent(302)
}

// CalculateVotes - Manual function to force vote recalculation.
func CalculateVotes(c echo.Context) error {
	var user = GetUser(c, true)

	if user.ID != 3 {
		SetToast(c, "notauth")
		return c.Redirect(302, "/")
	}

	query := `SELECT id, votes FROM beats`

	rows, err := db.Query(query)

	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	for rows.Next() {
		id := 0
		votes := 1
		rows.Scan(&id, &votes)

		err := db.QueryRow("SELECT COUNT(votecount.id) FROM votes AS votecount WHERE votecount.beat_id=?", id).Scan(&votes)

		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		updateQuery := "UPDATE beats SET votes = ? WHERE id = ?"

		upd, err := db.Prepare(updateQuery)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}
		defer upd.Close()

		upd.Exec(votes, id)
	}
	if err = rows.Err(); err != nil {
		// handle the error here
	}
	if err = rows.Close(); err != nil {
		// but what should we do if there's an error?
		log.Println(err)
	}

	return c.NoContent(302)
}

// AddVote ...
func AddVote(c echo.Context) error {
	user := GetUser(c, true)
	if !user.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	beatID, err := strconv.Atoi(c.FormValue("id"))
	if err != nil {
		println("test")
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
	if beatUserID == user.ID {
		return AjaxResponse(c, false, redirectURL, "owntrack")
	}

	count := 0
	_ = db.QueryRow("SELECT COUNT(id) FROM votes WHERE user_id = ? AND challenge_id = ?", user.ID, battleID).Scan(&count)

	voteID := 0
	voteErr := db.QueryRow("SELECT id FROM votes WHERE user_id = ? AND beat_id = ?", user.ID, beatID).Scan(&voteID)

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
			ins.Exec(beatID, user.ID, battleID)

			// Mark user as having voted if they've entered the battle themselves.
			votedSQL := "UPDATE beats SET voted = 1 WHERE user_id = ? AND challenge_id = ?"
			voted, _ := tx.Prepare(votedSQL)
			defer voted.Close()
			voted.Exec(user.ID, battleID)

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
				voted.Exec(user.ID, battleID)
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
			voted.Exec(user.ID, battleID)
		}

		// Commit and return deleted vote.
		tx.Commit()
		return AjaxResponse(c, false, redirectURL, "successdelvote")
	}

	return AjaxResponse(c, false, redirectURL, "404")
}

// AddLike ...
func AddLike(c echo.Context) error {
	user := GetUser(c, true)
	if !user.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	beatID, err := strconv.Atoi(c.FormValue("id"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	var battleID int
	var beatUserID int

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &beatUserID)
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if !RowExists("SELECT user_id FROM likes WHERE user_id = ? AND beat_id = ?", user.ID, beatID) {
		ins, err := db.Prepare("INSERT INTO likes(user_id, beat_id) VALUES (?, ?)")
		if err != nil {
			return AjaxResponse(c, true, "/", "502")
		}
		defer ins.Close()
		ins.Exec(user.ID, beatID)
		return AjaxResponse(c, false, redirectURL, "liked")
	}

	del, err := db.Prepare("DELETE from likes WHERE user_id = ? AND beat_id = ?")
	if err != nil {
		return AjaxResponse(c, true, "/", "502")
	}
	defer del.Close()
	del.Exec(user.ID, beatID)

	return AjaxResponse(c, false, redirectURL, "unliked")
}

// AddFeedback ...
func AddFeedback(c echo.Context) error {
	user := GetUser(c, true)
	if !user.Authenticated {
		return AjaxResponse(c, true, "/login/", "noauth")
	}

	beatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	var battleID int
	var beatUserID int
	feedback := policy.Sanitize(c.FormValue("feedback"))

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &beatUserID)
	if err != nil {
		return AjaxResponse(c, true, "/", "404")
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if beatUserID == user.ID {
		return AjaxResponse(c, false, "/", "feedbackself")
	}

	if !RowExists("SELECT id FROM feedback WHERE user_id = ? AND beat_id = ?", user.ID, beatID) {
		ins, err := db.Prepare("INSERT INTO feedback(feedback, user_id, beat_id) VALUES (?, ?, ?)")
		if err != nil {
			return AjaxResponse(c, true, "/", "502")
		}
		defer ins.Close()
		ins.Exec(feedback, user.ID, beatID)
		return AjaxResponse(c, false, redirectURL, "successaddfeedback")
	}

	update, err := db.Prepare("UPDATE feedback SET feedback = ? WHERE user_id = ? AND beat_id = ?")
	if err != nil {
		return AjaxResponse(c, true, "/", "502")
	}
	defer update.Close()
	update.Exec(feedback, user.ID, beatID)

	return AjaxResponse(c, false, redirectURL, "successupdate")
}

// ViewFeedback - Retreives battle and displays to user.
func ViewFeedback(c echo.Context) error {
	toast := GetToast(c)

	user := GetUser(c, true)
	if !user.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

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

	rows, err := db.Query(query, battleID, user.ID)
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
	if err = rows.Err(); err != nil {
		// handle the error here
	}
	if err = rows.Close(); err != nil {
		// but what should we do if there's an error?
		log.Println(err)
	}

	feedbackJSON, err := json.Marshal(feedback)

	m := map[string]interface{}{
		"Title":    battle.Title,
		"Battle":   battle,
		"Feedback": string(feedbackJSON),
		"User":     user,
		"Toast":    toast,
	}

	return c.Render(302, "Feedback", m)
}

// UserAccount - Retrieves all of user's battles and displays to user.
func UserAccount(c echo.Context) error {
	toast := GetToast(c)

	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	user := GetUser(c, false)

	userGroups := []Group{}
	if user.Authenticated {
		userGroups = GetGroupsByRole(db, user.ID, "owner")
	}

	nickname := ""
	err = db.QueryRow("SELECT nickname FROM users WHERE id = ?", userID).Scan(&nickname)
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	battles := GetBattles("challenges.user_id", strconv.Itoa(userID))
	battlesJSON, _ := json.Marshal(battles)

	m := map[string]interface{}{
		"Title":      nickname + "'s Battles",
		"Battles":    string(battlesJSON),
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Tag":        policy.Sanitize(c.Param("tag")),
		"UserID":     userID,
		"Nickname":   nickname,
	}

	return c.Render(302, "UserAccount", m)
}

// UserSubmissions - Retrieves all of user's battles and displays to user.
func UserSubmissions(c echo.Context) error {
	toast := GetToast(c)

	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	user := GetUser(c, false)

	userGroups := []Group{}
	if user.Authenticated {
		userGroups = GetGroupsByRole(db, user.ID, "owner")
	}

	nickname := ""
	err = db.QueryRow("SELECT nickname FROM users WHERE id = ?", userID).Scan(&nickname)
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	submission := Beat{}
	entries := []Beat{}

	query := `
			SELECT beats.url, beats.votes, voted.id IS NOT NULL AS voted, challenges.id, challenges.status, challenges.title
			FROM beats 
			LEFT JOIN votes AS voted on voted.user_id=beats.user_id AND voted.challenge_id=beats.challenge_id
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
		voted := 0
		submission = Beat{}
		err = rows.Scan(&submission.URL, &submission.Votes, &voted, &submission.ChallengeID, &submission.Status, &submission.Battle)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		submission.Status = strings.Title(submission.Status)

		if voted == 0 {
			submission.Status = `<span class="tooltipped" data-tooltip="Did Not Vote">` + submission.Status + ` <span style="color: #1E19FF;">(*)</span></span>`
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
	if err = rows.Err(); err != nil {
		// handle the error here
	}
	if err = rows.Close(); err != nil {
		// but what should we do if there's an error?
		log.Println(err)
	}

	submissionsJSON, err := json.Marshal(entries)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	m := map[string]interface{}{
		"Title":      nickname + "'s Submissions",
		"Beats":      string(submissionsJSON),
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Tag":        policy.Sanitize(c.Param("tag")),
		"UserID":     userID,
		"IsMobile":   isMobile,
		"Nickname":   nickname,
	}

	return c.Render(302, "UserSubmissions", m)
}

// TODO - USER AND ME CAN BE CONSOLIDATED INTO ONE REQUEST WITH A BOOLEAN FOR ACCESS

// UserGroups - Retrieves all of user's groups and displays to user.
func UserGroups(c echo.Context) error {

	toast := GetToast(c)

	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	user := GetUser(c, false)

	userGroups := []Group{}
	if user.Authenticated {
		userGroups = GetGroupsByRole(db, user.ID, "owner")
	}

	nickname := ""
	err = db.QueryRow("SELECT nickname FROM users WHERE id = ?", userID).Scan(&nickname)
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	groups := GetGroups(db, userID)
	groupsJSON, _ := json.Marshal(groups)

	m := map[string]interface{}{
		"Title":      nickname + "'s Groups",
		"Groups":     string(groupsJSON),
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"UserID":     userID,
		"Nickname":   nickname,
	}

	return c.Render(302, "UserGroups", m)
}

// MyAccount - Retrieves all of user's battles and displays to user.
func MyAccount(c echo.Context) error {
	toast := GetToast(c)
	user := GetUser(c, false)

	battles := GetBattles("challenges.user_id", strconv.Itoa(user.ID))
	battlesJSON, _ := json.Marshal(battles)

	m := map[string]interface{}{
		"Title":   "My Battles",
		"Battles": string(battlesJSON),
		"User":    user,
		"Toast":   toast,
		"Tag":     policy.Sanitize(c.Param("tag")),
	}

	return c.Render(302, "MyAccount", m)
}

// MySubmissions - Retrieves all of user's battles and displays to user.
func MySubmissions(c echo.Context) error {

	toast := GetToast(c)

	user := GetUser(c, false)
	if !user.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	submission := Beat{}
	entries := []Beat{}

	query := `
			SELECT beats.url, beats.votes, voted.id IS NOT NULL AS voted, challenges.id, challenges.status, challenges.title
			FROM beats 
			LEFT JOIN votes AS voted on voted.user_id=beats.user_id AND voted.challenge_id=beats.challenge_id
			LEFT JOIN challenges on challenges.id=beats.challenge_id
			WHERE beats.user_id=?
			GROUP BY 1
			ORDER BY beats.id DESC`

	rows, err := db.Query(query, user.ID)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer rows.Close()

	ua := c.Request().Header.Get("User-Agent")
	mobileUA := regexp.MustCompile(`/Mobile|Android|BlackBerry|iPhone/`)
	isMobile := mobileUA.MatchString(ua)

	for rows.Next() {
		voted := 0
		submission = Beat{}
		err = rows.Scan(&submission.URL, &submission.Votes, &voted, &submission.ChallengeID, &submission.Status, &submission.Battle)
		if err != nil {
			SetToast(c, "502")
			return c.Redirect(302, "/")
		}

		submission.Status = strings.Title(submission.Status)

		if voted == 0 {
			submission.Status = `<span class="tooltipped" data-tooltip="Did Not Vote">` + submission.Status + ` <span style="color: #1E19FF;">(*)</span></span>`
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
	if err = rows.Err(); err != nil {
		// handle the error here
	}
	if err = rows.Close(); err != nil {
		// but what should we do if there's an error?
		log.Println(err)
	}

	submissionsJSON, err := json.Marshal(entries)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	m := map[string]interface{}{
		"Title":    "My Submissions",
		"Beats":    string(submissionsJSON),
		"User":     user,
		"Toast":    toast,
		"IsMobile": isMobile,
		"Tag":      policy.Sanitize(c.Param("tag")),
	}

	return c.Render(302, "MySubmissions", m)
}

// MyGroups - Retrieves all of user's groups and displays to user.
func MyGroups(c echo.Context) error {

	toast := GetToast(c)

	user := GetUser(c, false)
	if !user.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	requests, invites, groups := GetUserGroups(db, user.ID)

	requestsJSON, _ := json.Marshal(requests)
	invitesJSON, _ := json.Marshal(invites)
	groupsJSON, _ := json.Marshal(groups)

	requestsString := string(requestsJSON)
	invitesString := string(invitesJSON)
	groupsString := string(groupsJSON)

	if requestsString == "[]" {
		requestsString = ""
	}

	if invitesString == "[]" {
		invitesString = ""
	}

	if groupsString == "[]" {
		groupsString = ""
	}

	m := map[string]interface{}{
		"Title":    "My Groups",
		"Requests": requestsString,
		"Invites":  invitesString,
		"Groups":   groupsString,
		"User":     user,
		"Toast":    toast,
	}

	return c.Render(302, "MyGroups", m)
}
