package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/markbates/goth/gothic"
)

// Callback ...
func Callback(w http.ResponseWriter, r *http.Request) {
	// STORE SESSION TOKEN & AUTH TOKEN, VERIFY VALID
	session, err := store.Get(r, "beatbattle")
	if err != nil {
		session.Options.MaxAge = -1
		err = session.Save(r, w)
		http.Redirect(w, r, "/login/cache", 302)
		return
	}

	Account := User{}

	handler := r.URL.Query().Get(":provider")
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
		Account.Authenticated = true
	}

	defer r.Body.Close()

	db := dbConn()
	defer db.Close()

	userID := 0
	err = db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/", 302)
		return
	}

	// If user doesn't exist, add to db
	// TODO UPDATE NICKNAME
	if userID == 0 {
		sql := "INSERT INTO users(provider, provider_id, nickname) VALUES(?,?,?)"

		stmt, err := db.Prepare(sql)
		if err != nil {
			http.Redirect(w, r, "/login/cache", 302)
			return
		}
		defer stmt.Close()

		stmt.Exec(Account.Provider, Account.ProviderID, Account.Name)
	} else {
		sql := "UPDATE users SET nickname = ? WHERE id = ?"

		stmt, err := db.Prepare(sql)
		if err != nil {
			http.Redirect(w, r, "/login/cache", 302)
			return
		}
		defer stmt.Close()

		stmt.Exec(Account.Provider, Account.ProviderID, Account.Name)
	}

	err = db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/", 302)
		return
	}

	Account.ID = userID
	print(userID)
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

// User struct.
type User struct {
	ID            int
	Provider      string
	ProviderID    string
	Name          string
	Avatar        string
	Authenticated bool
}

// GetUser ...
func GetUser(res http.ResponseWriter, req *http.Request) User {
	var user User
	user.ID = 0

	session, err := store.Get(req, "beatbattle")
	if err != nil {
		session, err = store.New(req, "beatbattle")
		if err != nil {
			http.Redirect(res, req, "/login/cache", 302)
			return user
		}
		session.Values["user"] = User{}
		err = session.Save(req, res)
		if err != nil {
			http.Redirect(res, req, "/login/cache", 302)
			return user
		}
	}

	if session.Values["user"] != nil {
		user = session.Values["user"].(User)
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

// AddVote ...
func AddVote(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	ajax := r.Header.Get("X-Requested-With") == "xmlhttprequest"

	user := GetUser(w, r)
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
			stmt, err := tx.Prepare(sql)
			if err != nil {
				AjaxResponse(w, r, true, ajax, redirectURL, "404")
				return
			}
			defer stmt.Close()

			stmt.Exec(beatID, user.ID, battleID)
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
	tx.Commit()

	AjaxResponse(w, r, false, ajax, redirectURL, "successdelvote")
	return
}

// AddLike ...
func AddLike(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	ajax := r.Header.Get("X-Requested-With") == "xmlhttprequest"

	user := GetUser(w, r)
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

	if !RowExists(db, "SELECT user_id FROM likes WHERE user_id = ? AND beat_id = ?", user.ID, beatID) {
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
	db := dbConn()
	defer db.Close()

	ajax := r.Header.Get("X-Requested-With") == "xmlhttprequest"

	user := GetUser(w, r)
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

	if !RowExists(db, "SELECT id FROM feedback WHERE user_id = ? AND beat_id = ?", user.ID, beatID) {
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
func ViewFeedback(wr http.ResponseWriter, req *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(req.URL.Query().Get(":toast"))

	user := GetUser(wr, req)
	if !user.Authenticated {
		http.Redirect(wr, req, "/login/noauth", 302)
		return
	}

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

	query := `SELECT users.nickname, feedback.feedback
				FROM beats
				LEFT JOIN feedback on feedback.beat_id = beats.id
				LEFT JOIN users on feedback.user_id = users.id
				WHERE beats.challenge_id = ? AND beats.user_id = ? AND feedback.feedback IS NOT NULL`

	rows, err := db.Query(query, battleID, user.ID)
	if err != nil {
		// This doesn't crash anything, but should be avoided.
		fmt.Println(err)
		http.Redirect(wr, req, "/404", 302)
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
			fmt.Println(err)
			http.Redirect(wr, req, "/502", 302)
			return
		}

		feedback = append(feedback, curFeedback)
	}

	e, err := json.Marshal(feedback)
	if err != nil {
		fmt.Println(err)
		return
	}

	m := map[string]interface{}{
		"Title":    battle.Title,
		"Battle":   battle,
		"Feedback": string(e),
		"User":     user,
		"Toast":    toast,
	}

	tmpl.ExecuteTemplate(wr, "Feedback", m)
}
