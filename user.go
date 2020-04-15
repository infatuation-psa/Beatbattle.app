package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/markbates/goth/gothic"
)

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

	db := dbConn()
	defer db.Close()

	userID := 0
	err = db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/", 302)
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
	m := map[string]interface{}{
		"Title": "Login",
		"Toast": toast,
	}
	tmpl.ExecuteTemplate(w, "Login", m)
}

// Auth ...
func Auth(w http.ResponseWriter, r *http.Request) {
	handler := r.URL.Query().Get(":provider")
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
	}

	session.Options.MaxAge = -1

	err = session.Save(r, w)
	if err != nil {
		http.Redirect(w, r, "/login/cache", 302)
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
		if strings.Contains(err.Error(), "The system cannot find the file specified.") || strings.Contains(err.Error(), "could not find a matching session for this request") {
			session.Values["user"] = User{}

			err = session.Save(req, res)
			if err != nil {
				http.Error(res, err.Error(), http.StatusInternalServerError)
			}
		} else {
			http.Error(res, err.Error(), http.StatusInternalServerError)
		}
	}

	if session.Values["user"] != nil {
		user = session.Values["user"].(User)
	}

	return user
}

// AddVote ...
func AddVote(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}

	beatID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	var battleID int
	var beatUserID int

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE id = ?", beatID).Scan(&battleID, &beatUserID)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	// Reject if beat is invalid.
	if err == sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID)

	// Get Battle status & max votes.
	status := ""
	maxVotes := 1
	err = db.QueryRow("SELECT status, maxvotes FROM challenges WHERE id = ?", battleID).Scan(&status, &maxVotes)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	// Reject if not currently in voting stage or if challenge is invalid.
	if err == sql.ErrNoRows || status != "voting" {
		http.Redirect(w, r, redirectURL+"/notvoting", 302)
		return
	}

	// Reject if user ID matches the track.
	if beatUserID == user.ID {
		http.Redirect(w, r, redirectURL+"/owntrack", 302)
		return
	}

	count := 0
	err = db.QueryRow("SELECT COUNT(id) FROM votes WHERE user_id = ? AND challenge_id = ?", user.ID, battleID).Scan(&count)

	voteID := 0
	err = db.QueryRow("SELECT id FROM votes WHERE user_id = ? AND beat_id = ?", user.ID, beatID).Scan(&voteID)

	if count < maxVotes {
		if err == sql.ErrNoRows {
			tx, err := db.Begin()
			if err != nil {
				panic(err.Error())
			}
			sql := "INSERT INTO votes(beat_id, user_id, challenge_id) VALUES(?,?,?)"
			stmt, err := tx.Prepare(sql)
			if err != nil {
				panic(err.Error())
			}
			defer stmt.Close()

			stmt.Exec(beatID, user.ID, battleID)

			sql = "UPDATE beats SET votes = votes + 1 WHERE id = ?"

			stmt, err = tx.Prepare(sql)
			if err != nil {
				panic(err.Error())
			}
			defer stmt.Close()

			stmt.Exec(beatID)
			tx.Commit()
			http.Redirect(w, r, redirectURL+"/successvote", 302)
			return
		} else if err != nil {
			panic(err.Error())
		}
	} else {
		if err == sql.ErrNoRows {
			http.Redirect(w, r, redirectURL+"/maxvotes", 302)
			return
		}
	}

	tx, err := db.Begin()
	sql := "DELETE FROM votes WHERE id = ?"

	stmt, err := tx.Prepare(sql)
	if err != nil {
		panic(err.Error())
	}
	defer stmt.Close()

	stmt.Exec(voteID)

	sql = "UPDATE beats SET votes = votes - 1 WHERE id = ?"

	stmt, err = tx.Prepare(sql)
	if err != nil {
		panic(err.Error())
	}
	defer stmt.Close()

	stmt.Exec(beatID)

	tx.Commit()

	http.Redirect(w, r, redirectURL+"/successdelvote", 302)
	return
}
