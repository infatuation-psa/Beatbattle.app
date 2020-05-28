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
func Callback(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "beatbattle")
	if err != nil {
		session.Options.MaxAge = -1
		err = session.Save(r, w)
		http.Redirect(w, r, "/login/cache", 302)
		return
	}

	Account := User{}

	handler := r.URL.Query().Get(":provider")
	defer r.Body.Close()

	if handler != "reddit" {
		user, err := gothic.CompleteUserAuth(w, r)
		if err != nil {
			session.Options.MaxAge = -1
			err = session.Save(r, w)
			http.Redirect(w, r, "/login/cache", 302)
			return
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
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		token, err := redditAuth.GetToken(state, code)
		if err != nil {
			session.Options.MaxAge = -1
			err = session.Save(r, w)
			http.Redirect(w, r, "/login/cache", 302)
			return
		}
		client := redditAuth.GetAuthClient(token)
		user, err := client.GetMe()
		if err != nil {
			session.Options.MaxAge = -1
			err = session.Save(r, w)
			http.Redirect(w, r, "/login/cache", 302)
			return
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
	err = db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/", 302)
		return
	}

	accessTokenEncrypted := HashAndSalt([]byte(Account.AccessToken))
	// If user doesn't exist, add to db
	if userID == 0 {
		sql := "INSERT INTO users(provider, provider_id, nickname, access_token, expiry) VALUES(?,?,?,?,?)"

		stmt, err := db.Prepare(sql)
		if err != nil {
			http.Redirect(w, r, "/login/cache", 302)
			return
		}
		defer stmt.Close()

		stmt.Exec(Account.Provider, Account.ProviderID, Account.Name, accessTokenEncrypted, Account.ExpiresAt)
	} else {
		sql := "UPDATE users SET nickname = ?, access_token = ?, expiry = ? WHERE id = ?"

		stmt, err := db.Prepare(sql)
		if err != nil {
			http.Redirect(w, r, "/login/cache", 302)
			return
		}
		defer stmt.Close()

		stmt.Exec(Account.Name, accessTokenEncrypted, Account.ExpiresAt, userID)
	}

	err = db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/", 302)
		return
	}

	Account.ID = userID
	session.Values["user"] = Account

	err = session.Save(r, w)
	if err != nil {
		session.Options.MaxAge = -1
		err = session.Save(r, w)
		http.Redirect(w, r, "/login/cache", 302)
		return
	}

	http.Redirect(w, r, "/", 302)
}

// Login ...
func Login(w http.ResponseWriter, r *http.Request) {
	toast := GetToast(r.URL.Query().Get(":toast"))
	defer r.Body.Close()

	m := map[string]interface{}{
		"Title": "Login",
		"Toast": toast,
	}
	tmpl.ExecuteTemplate(w, "Login", m)
}

// Auth ...
func Auth(w http.ResponseWriter, r *http.Request) {
	handler := r.URL.Query().Get(":provider")
	defer r.Body.Close()

	if handler == "reddit" {
		http.Redirect(w, r, redditAuth.GetAuthenticationURL(), 307)
		return
	}

	if gothUser, err := gothic.CompleteUserAuth(w, r); err == nil {
		tmpl.ExecuteTemplate(w, "UserTemplate", gothUser)
	} else {
		gothic.BeginAuthHandler(w, r)
	}
}

// Logout ...
func Logout(w http.ResponseWriter, r *http.Request) {
	gothic.Logout(w, r)

	session, err := store.Get(r, "beatbattle")
	if err != nil {
		http.Redirect(w, r, "/login/cache", 302)
		return
	}

	session.Options.MaxAge = -1

	err = session.Save(r, w)
	if err != nil {
		http.Redirect(w, r, "/login/cache", 302)
		return
	}

	http.Redirect(w, r, "/", 302)
}

// GenericLogout ...
func GenericLogout(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "beatbattle")
	if err != nil {
		http.Redirect(w, r, "/login/cache", 302)
		return
	}

	session.Options.MaxAge = -1

	err = session.Save(r, w)
	if err != nil {
		http.Redirect(w, r, "/login/cache", 302)
		return
	}

	http.Redirect(w, r, "/", 302)
}

// GetUser ...
func GetUser(res http.ResponseWriter, req *http.Request, validate bool) User {
	var user User
	user.ID = 0

	session, err := store.Get(req, "beatbattle")
	if err != nil {
		session, err = store.New(req, "beatbattle")
		if err != nil {
			http.Redirect(res, req, "/login/cache", 302)
			return User{}
		}
		session.Values["user"] = User{}
		err = session.Save(req, res)
		if err != nil {
			http.Redirect(res, req, "/login/cachesave", 302)
			return User{}
		}
	}

	if session.Values["user"] != nil {
		user = session.Values["user"].(User)

		// Kick out users who haven't logged in since the update.
		if user.AccessToken == "" {
			session.Values["user"] = User{}
			err = session.Save(req, res)
			if err != nil {
				http.Redirect(res, req, "/login/relog", 302)
			}
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
						session.Values["user"] = User{}
						err = session.Save(req, res)
						if err != nil {
							http.Redirect(res, req, "/login/relog", 302)
						}
						return User{}
					}
				}

				if user.Provider == "reddit" {
					// TODO - Refresh reddit token
					return user
					/*
						newToken, err = redditAuth.GetToken(state, user.RefreshToken)
						if err != nil {
							session.Values["user"] = User{}
							err = session.Save(req, res)
							if err != nil {
								http.Redirect(res, req, "/login/relog", 302)
							}
							return User{}
						}
					*/
				}

				user.AccessToken = newToken.AccessToken
				user.RefreshToken = newToken.RefreshToken
				user.ExpiresAt = newToken.Expiry

				sql := "UPDATE users SET access_token = ?, expiry = ? WHERE id = ?"

				stmt, err := db.Prepare(sql)
				if err != nil {
					http.Redirect(res, req, "/login/cache", 302)
					return User{}
				}
				defer stmt.Close()

				accessTokenEncrypted := HashAndSalt([]byte(user.AccessToken))
				dbHash = accessTokenEncrypted
				stmt.Exec(accessTokenEncrypted, user.ExpiresAt, user.ID)
			}

			if !comparePasswords(dbHash, []byte(user.AccessToken)) {
				session.Values["user"] = User{}
				err = session.Save(req, res)
				if err != nil {
					http.Redirect(res, req, "/login/relog", 302)
				}
				return User{}
			}
		}
	}

	return user
}

// AjaxResponse ...
func AjaxResponse(w http.ResponseWriter, r *http.Request, redirect bool, ajax bool, redirectPath string, toastQuery string) {
	// TODO - FUCK YOU IF YOU DON'T HAVE JAVASCRIPT, NOT NECESSARY
	type AjaxData struct {
		Redirect   string
		ToastHTML  string
		ToastClass string
		ToastQuery string
	}

	if redirect && !ajax {
		http.Redirect(w, r, redirectPath+toastQuery, 302)
		return
	}

	toast := GetToast(toastQuery)
	data := AjaxData{ToastHTML: toast[0], ToastClass: toast[1], ToastQuery: toastQuery}

	if ajax {
		if redirect {
			data.Redirect = redirectPath + toastQuery
		}
		json.NewEncoder(w).Encode(data)
		return
	}

	http.Redirect(w, r, redirectPath+toastQuery, 302)
}

// CalculateVotes - Manual function to force vote recalculation.
func CalculateVotes(w http.ResponseWriter, r *http.Request) {
	var user = GetUser(w, r, true)

	if user.ID != 3 {
		http.Redirect(w, r, "/notauth", 302)
		return
	}

	query := `SELECT id, votes FROM beats`

	rows, err := db.Query(query)

	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer rows.Close()

	for rows.Next() {
		id := 0
		votes := 1
		rows.Scan(&id, &votes)

		err := db.QueryRow("SELECT COUNT(votecount.id) FROM votes AS votecount WHERE votecount.beat_id=?", id).Scan(&votes)

		if err != nil {
			http.Redirect(w, r, "/502", 302)
			return
		}

		updateQuery := "UPDATE beats SET votes = ? WHERE id = ?"

		upd, err := db.Prepare(updateQuery)
		if err != nil {
			http.Redirect(w, r, "/502", 302)
			return
		}
		defer upd.Close()

		upd.Exec(votes, id)
	}
}

// AddVote ...
func AddVote(w http.ResponseWriter, r *http.Request) {

	ajax := r.Header.Get("X-Requested-With") == "xmlhttprequest"

	user := GetUser(w, r, true)
	if !user.Authenticated {
		AjaxResponse(w, r, true, ajax, "/login/", "noauth")
		return
	}

	beatID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "404")
		return
	}
	defer r.Body.Close()

	var battleID int
	var beatUserID int

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &beatUserID)
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "404")
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	// Get Battle status & max votes.
	status := ""
	maxVotes := 1
	err = db.QueryRow("SELECT status, maxvotes FROM challenges WHERE id = ?", battleID).Scan(&status, &maxVotes)
	if err != nil && err != sql.ErrNoRows {
		AjaxResponse(w, r, true, ajax, "/", "502")
		return
	}

	// Reject if not currently in voting stage or if challenge is invalid.
	if err == sql.ErrNoRows || status != "voting" {
		AjaxResponse(w, r, true, ajax, redirectURL, "302")
		return
	}

	// Reject if user ID matches the track.
	if beatUserID == user.ID {
		AjaxResponse(w, r, false, ajax, redirectURL, "owntrack")
		return
	}

	count := 0
	err = db.QueryRow("SELECT COUNT(id) FROM votes WHERE user_id = ? AND challenge_id = ?", user.ID, battleID).Scan(&count)

	voteID := 0
	err = db.QueryRow("SELECT id FROM votes WHERE user_id = ? AND beat_id = ?", user.ID, beatID).Scan(&voteID)

	// TODO Change from transaction maybe

	if count < maxVotes {
		if err == sql.ErrNoRows {
			tx, err := db.Begin()
			if err != nil {
				AjaxResponse(w, r, true, ajax, redirectURL, "404")
				return
			}
			sql := "INSERT INTO votes(beat_id, user_id, challenge_id) VALUES(?,?,?)"
			ins, err := tx.Prepare(sql)
			if err != nil {
				AjaxResponse(w, r, true, ajax, redirectURL, "404")
				return
			}
			defer ins.Close()

			ins.Exec(beatID, user.ID, battleID)

			updSQL := "UPDATE beats SET votes = votes + 1 WHERE id = ?"
			upd, err := tx.Prepare(updSQL)
			if err != nil {
				AjaxResponse(w, r, true, ajax, redirectURL, "404")
				return
			}
			defer upd.Close()

			upd.Exec(beatID)
			tx.Commit()

			AjaxResponse(w, r, false, ajax, redirectURL, "successvote")
			return
		} else if err != nil {
			AjaxResponse(w, r, true, ajax, redirectURL, "404")
			return
		}
	} else {
		if err == sql.ErrNoRows {
			AjaxResponse(w, r, false, ajax, redirectURL, "maxvotes")
			return
		}
	}

	tx, err := db.Begin()
	sql := "DELETE FROM votes WHERE id = ?"

	stmt, err := tx.Prepare(sql)
	if err != nil {
		AjaxResponse(w, r, true, ajax, redirectURL, "404")
		return
	}
	defer stmt.Close()

	stmt.Exec(voteID)

	updSQL := "UPDATE beats SET votes = votes - 1 WHERE id = ?"
	upd, err := tx.Prepare(updSQL)
	if err != nil {
		AjaxResponse(w, r, true, ajax, redirectURL, "404")
		return
	}
	defer upd.Close()

	upd.Exec(beatID)
	tx.Commit()

	AjaxResponse(w, r, false, ajax, redirectURL, "successdelvote")
	return
}

// AddLike ...
func AddLike(w http.ResponseWriter, r *http.Request) {

	ajax := r.Header.Get("X-Requested-With") == "xmlhttprequest"

	user := GetUser(w, r, true)
	if !user.Authenticated {
		AjaxResponse(w, r, true, ajax, "/login/", "noauth")
		return
	}

	beatID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "404")
		return
	}
	defer r.Body.Close()

	var battleID int
	var beatUserID int

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &beatUserID)
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "404")
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if !RowExists("SELECT user_id FROM likes WHERE user_id = ? AND beat_id = ?", user.ID, beatID) {
		ins, err := db.Prepare("INSERT INTO likes(user_id, beat_id) VALUES (?, ?)")
		if err != nil {
			AjaxResponse(w, r, true, ajax, "/", "502")
			return
		}
		defer ins.Close()
		ins.Exec(user.ID, beatID)
		AjaxResponse(w, r, false, ajax, redirectURL, "liked")
		return
	}

	del, err := db.Prepare("DELETE from likes WHERE user_id = ? AND beat_id = ?")
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "502")
		return
	}
	defer del.Close()
	del.Exec(user.ID, beatID)

	AjaxResponse(w, r, false, ajax, redirectURL, "unliked")
	return
}

// AddFeedback ...
func AddFeedback(w http.ResponseWriter, r *http.Request) {

	ajax := r.Header.Get("X-Requested-With") == "xmlhttprequest"

	user := GetUser(w, r, true)
	if !user.Authenticated {
		AjaxResponse(w, r, true, ajax, "/login/", "noauth")
		return
	}

	beatID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "404")
		return
	}
	defer r.Body.Close()

	var battleID int
	var beatUserID int
	feedback := policy.Sanitize(r.FormValue("feedback"))

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &beatUserID)
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "404")
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID) + "/"

	if beatUserID == user.ID {
		AjaxResponse(w, r, false, ajax, "/", "feedbackself")
		return
	}

	if !RowExists("SELECT id FROM feedback WHERE user_id = ? AND beat_id = ?", user.ID, beatID) {
		ins, err := db.Prepare("INSERT INTO feedback(feedback, user_id, beat_id) VALUES (?, ?, ?)")
		if err != nil {
			AjaxResponse(w, r, true, ajax, "/", "502")
			return
		}
		defer ins.Close()
		ins.Exec(feedback, user.ID, beatID)
		AjaxResponse(w, r, false, ajax, redirectURL, "successaddfeedback")
		return
	}

	update, err := db.Prepare("UPDATE feedback SET feedback = ? WHERE user_id = ? AND beat_id = ?")
	if err != nil {
		AjaxResponse(w, r, true, ajax, "/", "502")
		return
	}
	defer update.Close()
	update.Exec(feedback, user.ID, beatID)

	AjaxResponse(w, r, false, ajax, redirectURL, "successupdate")
	return
}

// ViewFeedback - Retreives battle and displays to user.
func ViewFeedback(w http.ResponseWriter, r *http.Request) {

	toast := GetToast(r.URL.Query().Get(":toast"))

	user := GetUser(w, r, true)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/relog", 302)
		return
	}

	battleID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}

	// Retrieve battle, return to front page if battle doesn't exist.
	battle := GetBattle(battleID)

	if battle.Title == "" {
		http.Redirect(w, r, "/404", 302)
		return
	}

	query := `SELECT users.nickname, feedback.feedback
				FROM beats
				LEFT JOIN feedback on feedback.beat_id = beats.id
				LEFT JOIN users on feedback.user_id = users.id
				WHERE beats.challenge_id = ? AND beats.user_id = ? AND feedback.feedback IS NOT NULL`

	rows, err := db.Query(query, battleID, user.ID)
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
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
			http.Redirect(w, r, "/502", 302)
			return
		}

		feedback = append(feedback, curFeedback)
	}

	e, err := json.Marshal(feedback)
	if err != nil {
		return
	}

	m := map[string]interface{}{
		"Title":    battle.Title,
		"Battle":   battle,
		"Feedback": string(e),
		"User":     user,
		"Toast":    toast,
	}

	tmpl.ExecuteTemplate(w, "Feedback", m)
}

// UserAccount - Retrieves all of user's battles and displays to user.
func UserAccount(w http.ResponseWriter, r *http.Request) {

	toast := GetToast(r.URL.Query().Get(":toast"))

	userID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}
	defer r.Body.Close()

	user := GetUser(w, r, false)
	if userID == user.ID {
		http.Redirect(w, r, "/me", 302)
		return
	}

	userGroups := []Group{}
	if user.Authenticated {
		userGroups = GetGroupsByRole(db, user.ID, "owner")
	}

	nickname := ""
	err = db.QueryRow("SELECT nickname FROM users WHERE id = ?", userID).Scan(&nickname)
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	battles := GetBattles("challenges.user_id", strconv.Itoa(userID))

	battlesJSON, err := json.Marshal(battles)
	if err != nil {
		return
	}

	m := map[string]interface{}{
		"Title":      nickname + "'s Battles",
		"Battles":    string(battlesJSON),
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Tag":        policy.Sanitize(r.URL.Query().Get(":tag")),
		"UserID":     userID,
		"Nickname":   nickname,
	}

	tmpl.ExecuteTemplate(w, "UserAccount", m)
}

// UserSubmissions - Retrieves all of user's battles and displays to user.
func UserSubmissions(w http.ResponseWriter, r *http.Request) {

	toast := GetToast(r.URL.Query().Get(":toast"))

	userID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}
	defer r.Body.Close()

	user := GetUser(w, r, false)
	if userID == user.ID {
		http.Redirect(w, r, "/me", 302)
		return
	}

	userGroups := []Group{}
	if user.Authenticated {
		userGroups = GetGroupsByRole(db, user.ID, "owner")
	}

	nickname := ""
	err = db.QueryRow("SELECT nickname FROM users WHERE id = ?", userID).Scan(&nickname)
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
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
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer rows.Close()

	ua := r.Header.Get("User-Agent")
	mobileUA := regexp.MustCompile(`/Mobile|Android|BlackBerry/`)
	isMobile := mobileUA.MatchString(ua)

	for rows.Next() {
		voted := 0
		submission = Beat{}
		err = rows.Scan(&submission.URL, &submission.Votes, &voted, &submission.ChallengeID, &submission.Status, &submission.Battle)
		if err != nil {
			http.Redirect(w, r, "/502", 302)
			return
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

	submissionsJSON, err := json.Marshal(entries)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}

	m := map[string]interface{}{
		"Title":      nickname + "'s Submissions",
		"Beats":      string(submissionsJSON),
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"Tag":        policy.Sanitize(r.URL.Query().Get(":tag")),
		"UserID":     userID,
		"IsMobile":   isMobile,
		"Nickname":   nickname,
	}

	tmpl.ExecuteTemplate(w, "UserSubmissions", m)
}

// TODO - USER AND ME CAN BE CONSOLIDATED INTO ONE REQUEST WITH A BOOLEAN FOR ACCESS

// UserGroups - Retrieves all of user's groups and displays to user.
func UserGroups(w http.ResponseWriter, r *http.Request) {

	toast := GetToast(r.URL.Query().Get(":toast"))

	userID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}
	defer r.Body.Close()

	user := GetUser(w, r, false)
	if userID == user.ID {
		http.Redirect(w, r, "/me", 302)
		return
	}

	userGroups := []Group{}
	if user.Authenticated {
		userGroups = GetGroupsByRole(db, user.ID, "owner")
	}

	nickname := ""
	err = db.QueryRow("SELECT nickname FROM users WHERE id = ?", userID).Scan(&nickname)
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	groups := GetGroups(db, userID)

	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		return
	}

	m := map[string]interface{}{
		"Title":      nickname + "'s Groups",
		"Groups":     string(groupsJSON),
		"User":       user,
		"UserGroups": userGroups,
		"Toast":      toast,
		"UserID":     userID,
		"Nickname":   nickname,
	}

	tmpl.ExecuteTemplate(w, "UserGroups", m)
}

// MyAccount - Retrieves all of user's battles and displays to user.
func MyAccount(w http.ResponseWriter, r *http.Request) {

	toast := GetToast(r.URL.Query().Get(":toast"))

	user := GetUser(w, r, false)

	battles := GetBattles("challenges.user_id", strconv.Itoa(user.ID))

	battlesJSON, err := json.Marshal(battles)
	if err != nil {
		return
	}

	m := map[string]interface{}{
		"Title":   "My Battles",
		"Battles": string(battlesJSON),
		"User":    user,
		"Toast":   toast,
		"Tag":     policy.Sanitize(r.URL.Query().Get(":tag")),
	}

	tmpl.ExecuteTemplate(w, "MyAccount", m)
}

// MySubmissions - Retrieves all of user's battles and displays to user.
func MySubmissions(w http.ResponseWriter, r *http.Request) {

	toast := GetToast(r.URL.Query().Get(":toast"))

	user := GetUser(w, r, false)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/relog", 302)
		return
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
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer rows.Close()

	ua := r.Header.Get("User-Agent")
	mobileUA := regexp.MustCompile(`/Mobile|Android|BlackBerry/`)
	isMobile := mobileUA.MatchString(ua)

	for rows.Next() {
		voted := 0
		submission = Beat{}
		err = rows.Scan(&submission.URL, &submission.Votes, &voted, &submission.ChallengeID, &submission.Status, &submission.Battle)
		if err != nil {
			http.Redirect(w, r, "/502", 302)
			return
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

	submissionsJSON, err := json.Marshal(entries)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}

	m := map[string]interface{}{
		"Title":    "My Submissions",
		"Beats":    string(submissionsJSON),
		"User":     user,
		"Toast":    toast,
		"IsMobile": isMobile,
		"Tag":      policy.Sanitize(r.URL.Query().Get(":tag")),
	}

	tmpl.ExecuteTemplate(w, "MySubmissions", m)
}

// MyGroups - Retrieves all of user's groups and displays to user.
func MyGroups(w http.ResponseWriter, r *http.Request) {

	toast := GetToast(r.URL.Query().Get(":toast"))

	user := GetUser(w, r, false)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/relog", 302)
		return
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

	tmpl.ExecuteTemplate(w, "MyGroups", m)
}
