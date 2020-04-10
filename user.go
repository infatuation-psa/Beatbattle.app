package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/markbates/goth/gothic"
)

// Callback ...
func Callback(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "beatbattle")
	if err != nil {
		log.Println("SESSION ISSUE1")
		Logout(w, r)
		return
	}

	Account := User{}

	handler := r.URL.Query().Get(":provider")
	if handler != "reddit" {
		user, err := gothic.CompleteUserAuth(w, r)
		if err != nil {
			log.Println("SESSION ISSUE2")
			Logout(w, r)
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
		println(state)
		code := r.URL.Query().Get("code")
		println(code)
		token, err := redditAuth.GetToken(state, code)
		if err != nil {
			fmt.Print(err)
			log.Println("REDDIT ISSUE1")
			Logout(w, r)
			return
		}
		client := redditAuth.GetAuthClient(token)
		user, err := client.GetMe()
		if err != nil {
			log.Println("REDDIT ISSUE2")
			Logout(w, r)
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
		panic(err.Error())
	}

	// If user doesn't exist, add to db
	if userID == 0 {
		sql := "INSERT INTO users(provider, provider_id, nickname) VALUES(?,?,?)"

		stmt, err := db.Prepare(sql)
		if err != nil {
			panic(err.Error())
		}
		defer stmt.Close()

		stmt.Exec(Account.Provider, Account.ProviderID, Account.Name)

		err = db.QueryRow("SELECT id FROM users WHERE provider=? and provider_id=?", Account.Provider, Account.ProviderID).Scan(&userID)
		if err != nil {
			// Something is wrong lmao
			http.Redirect(w, r, "/", 301)
		}
	}

	Account.ID = userID
	session.Values["user"] = Account
	fmt.Print(Account)

	err = session.Save(r, w)
	if err != nil {
		log.Println("SESSION ISSUE3")
		Logout(w, r)
		return
	}

	// TODO - Save last url in a cookie and redirect to that instead.
	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusTemporaryRedirect)

	// debug - tmpl.ExecuteTemplate(res, "UserTemplate", user)
}

// Login ...
func Login(w http.ResponseWriter, r *http.Request) {
	tmpl.ExecuteTemplate(w, "Login", "")
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
func Logout(res http.ResponseWriter, req *http.Request) {
	gothic.Logout(res, req)

	session, err := store.Get(req, "beatbattle")
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	session.Values["user"] = User{}
	session.Options.MaxAge = -1

	err = session.Save(req, res)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	res.Header().Set("Location", "/")
	res.WriteHeader(http.StatusTemporaryRedirect)
}

// GenericLogout ...
func GenericLogout(res http.ResponseWriter, req *http.Request) {
	session, err := store.Get(req, "beatbattle")
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	session.Values["user"] = User{}
	session.Options.MaxAge = -1

	err = session.Save(req, res)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	res.Header().Set("Location", "/")
	res.WriteHeader(http.StatusTemporaryRedirect)
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
		http.Redirect(w, r, "/auth/discord", 301)
		return
	}

	beatID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/battle/", 301)
		return
	}

	var battleID int
	var userID int

	err = db.QueryRow("SELECT challenge_id, user_id FROM beats WHERE beat_id = ?", beatID).Scan(&battleID, &userID)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	// Reject if beat is invalid.
	if err == sql.ErrNoRows {
		http.Redirect(w, r, "/battle/", 301)
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID)

	// Get Battle status & max votes.
	status := ""
	maxVotes := 1
	err = db.QueryRow("SELECT status, maxVotes FROM challenges WHERE id = ?", battleID).Scan(&status, &maxVotes)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}

	// Reject if not currently in voting stage or if challenge is invalid.
	if err == sql.ErrNoRows || status != "voting" {
		http.Redirect(w, r, redirectURL, 301)
		return
	}

	// Reject if user ID matches the track.
	if userID == user.ID {
		http.Redirect(w, r, redirectURL, 301)
		return
	}

	var lastVotes []int
	rows, err := db.Query("SELECT beat_id FROM votes WHERE user_id = ? AND challenge_id = ?", user.ID, battleID)
	if err != nil && err != sql.ErrNoRows {
		panic(err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var curBeatID int
		err = rows.Scan(&curBeatID)
		if err != nil {
			panic(err.Error())
		}
		lastVotes = append(lastVotes, curBeatID)
	}

	removeVote := binarySearch(beatID, lastVotes)

	if !removeVote && len(lastVotes) >= maxVotes {
		// If this beat ID hasn't been voted for already and you're already at max votes, boot ya.
		http.Redirect(w, r, redirectURL, 301)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err.Error())
	}

	if user.Authenticated {
		userID := user.ID

		// Step 1: Modify vote table.
		{
			sql := "INSERT INTO votes(beat_id, challenge_id, user_id) VALUES(?,?,?)"

			if removeVote {
				sql = "DELETE FROM votes WHERE beat_id = ? AND challenge_id = ? AND user_id = ?"
			}

			stmt, err := tx.Prepare(sql)
			if err != nil {
				panic(err.Error())
			}
			defer stmt.Close()

			stmt.Exec(beatID, battleID, userID)
		}

		// Step 2: Remove vote from beat.
		if removeVote {
			stmt, err := tx.Prepare("UPDATE beats SET votes = votes - 1 WHERE beat_id = ?")
			if err != nil {
				panic(err.Error())
			}
			defer stmt.Close()

			stmt.Exec(beatID)
		}

		// Step 3: Add vote to beat.
		if !removeVote {
			sql := "UPDATE beats SET votes = votes + 1 WHERE beat_id = ?"

			stmt, err := tx.Prepare(sql)
			if err != nil {
				panic(err.Error())
			}
			defer stmt.Close()

			stmt.Exec(beatID)
		}

		tx.Commit()
	} else {
		// TODO - Redirect with alert for user.
		http.Redirect(w, r, redirectURL, 301)
		return
	}
	// TODO - Redirect with alert for user.
	http.Redirect(w, r, redirectURL, 301)
	return
}
