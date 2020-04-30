package main

import (
	"crypto/tls"
	"encoding/gob"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/microcosm-cc/bluemonday"

	"github.com/cameronstanley/go-reddit"
	_ "github.com/go-sql-driver/mysql"

	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/discord"

	"github.com/Masterminds/sprig"
	"github.com/joho/godotenv"
)

/*-------
Variables
-------*/

// store will hold all session data
var store *sessions.FilesystemStore
var redditAuth *reddit.Authenticator

// tmpl holds all parsed templates
var tmpl *template.Template
var policy *bluemonday.Policy

var whitelist []string

/*-------
Help Actions
--------*/

func init() {
	/*
	   Safety net for 'too many open files' issue on legacy code.
	   Set a sane timeout duration for the http.DefaultClient, to ensure idle connections are terminated.
	   Reference: https://stackoverflow.com/questions/37454236/net-http-server-too-many-open-files-error
	*/
	http.DefaultClient.Timeout = 5 * time.Second

	// loads values from .env into the system
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}

	whitelist = []string{"f1eightco-my.sharepoint.com", "sharepoint.com", "drive.google.com", "youtube.com", "bandcamp.com", "soundcloud.com", "sellfy.com", "onedrive.com", "dropbox.com", "mega.nz", "amazon.com/clouddrive", "filetransfer.io", "wetransfer.com", "we.tt"}
	policy = bluemonday.UGCPolicy()
	//policy.AllowStandardURLs()

	/*authKeyOne := securecookie.GenerateRandomKey(64)
	encryptionKeyOne := securecookie.GenerateRandomKey(32)*/

	authKeyOne := []byte(os.Getenv("SECURE_KEY64"))
	encryptionKeyOne := []byte(os.Getenv("SECURE_KEY32"))

	store = sessions.NewFilesystemStore(
		"sessions/",
		authKeyOne,
		encryptionKeyOne,
	)

	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   60 * 10080,
		HttpOnly: true,
	}

	gob.Register(User{})

	tmpl = template.Must(template.New("base").Funcs(sprig.FuncMap()).ParseGlob("templates/*"))
}

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

// StringWithCharset ...
func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// RandString ...
func RandString(length int) string {
	return StringWithCharset(length, charset)
}

func main() {
	tlsConfig := &tls.Config{
		// Causes servers to use Go's default ciphersuite preferences,
		// which are tuned to avoid attacks. Does nothing on clients.
		PreferServerCipherSuites: true,
		// Only use curves which have assembly implementations
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519, // Go 1.8 only
		},
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, // Go 1.8 only
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,   // Go 1.8 only
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,

			// Best disabled, as they don't provide Forward Secrecy,
			// but might be necessary for some clients
			// tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			// tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	router := pat.New()

	srv := &http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    tlsConfig,
		Handler:      router,
	}

	static := http.StripPrefix("/static/", http.FileServer(http.Dir("./static/")))
	state := string(RandString(16))
	redditAuth = reddit.NewAuthenticator(os.Getenv("REDDIT_KEY"), os.Getenv("REDDIT_SECRET"), os.Getenv("REDDIT_CALLBACK"),
		"linux:beatbattle:v1.0 (by /u/infatuationpsa)", state, reddit.ScopeIdentity)

	gothic.Store = sessions.NewCookieStore([]byte(os.Getenv("DISCORD_SECRET")))
	goth.UseProviders(discord.New(os.Getenv("DISCORD_KEY"), os.Getenv("DISCORD_SECRET"), os.Getenv("CALLBACK_URL"), discord.ScopeIdentify))

	router.PathPrefix("/static/").Handler(static)

	// Handlers for users & auth
	router.Get("/auth/{provider}/callback", Callback)
	router.Get("/auth/{provider}", Auth)
	router.Get("/logout/{provider}", Logout)
	router.Get("/logout", GenericLogout)
	router.Post("/feedback/{id}", AddFeedback)
	router.Post("/vote/{id}", AddVote)
	router.Get("/login/{toast}", Login)
	router.Get("/login", Login)
	router.Get("/myaccount", MyAccount)

	router.Get("/battles/{tag}", ViewTaggedBattles)

	// Battle
	router.Get("/battle/{id}/update/timezone/{region}/{country}", UpdateBattle) // Timezone
	router.Get("/battle/{id}/update/{toast}", UpdateBattle)                     // Toast
	router.Post("/battle/{id}/update", UpdateBattleDB)                          // Update in db
	router.Get("/battle/{id}/update", UpdateBattle)                             // Update page
	router.Get("/battle/{id}/delete", DeleteBattle)
	router.Get("/battle/{id}/feedback", ViewFeedback)

	router.Get("/battle/submit/{toast}", SubmitBattle)
	router.Post("/battle/submit", InsertBattle)
	router.Get("/battle/submit", SubmitBattle)
	router.Get("/battle/{id}/{toast}", BattleHTTP)
	router.Get("/battle/{id}", BattleHTTP)

	// Beat
	router.Get("/beat/{id}/submit/{toast}", SubmitBeat)
	router.Get("/beat/{id}/submit", SubmitBeat)
	router.Post("/beat/{id}/submit", InsertBeat)
	router.Get("/beat/{id}/update/{toast}", SubmitBeat)
	router.Post("/beat/{id}/update", UpdateBeat)
	router.Get("/beat/{id}/update", SubmitBeat)
	router.Get("/beat/{id}/delete", DeleteBeat)

	router.Get("/past", ViewBattles)
	router.Get("/{toast}", ViewBattles)
	router.Get("/", ViewBattles)

	http.Handle("/", router)

	log.Fatal(srv.ListenAndServeTLS("server.cert", "server.key"))
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

// GetToast serves toast text.
func GetToast(toast string) [2]string {
	html := ""
	class := ""
	switch message := toast; message {
	case "404":
		html = "Battle or beat not found."
		class = "toast-error"
	case "502":
		html = "Server error."
		class = "toast-error"
	case "password":
		html = "Incorrect password."
		class = "toast-error"
	case "unapprovedurl":
		html = "URL not on approved list."
		class = "toast-error"
	case "notopen":
		html = "That battle is not currently open."
		class = "toast-error"
	case "nobeat":
		html = "You haven't submitted a beat to this battle."
		class = "toast-error"
	case "noauth":
		html = "You need to be logged in to do that."
		class = "toast-error"
	case "notuser":
		html = "You're not allowed to do that."
		class = "toast-error"
	case "notvoting":
		html = "This battle isn't currently accepting votes."
		class = "toast-error"
	case "owntrack":
		html = "You can't vote for your own track."
		class = "toast-error"
	case "maxvotes":
		html = "You're at your max votes for this battle."
		class = "toast-error"
	case "deadlinebefore":
		html = "The deadline cannot be before right now."
		class = "toast-error"
	case "votedeadlinebefore":
		html = "The voting deadline cannot be before the deadline."
		class = "toast-error"
	case "maxvotesinvalid":
		html = "Max votes must be between 1 and 10."
		class = "toast-error"
	case "nodata":
		html = "No data received.."
		class = "toast-error"
	case "validationerror":
		html = "Validation error, please try again."
		class = "toast-error"
	case "maxbattles":
		html = "You can only have 3 active battles at once."
		class = "toast-error"
	case "titleexists":
		html = "You already have a battle with this title."
		class = "toast-error"
	case "sconly":
		html = "You must submit a SoundCloud link."
		class = "toast-error"
	case "cache":
		html = "If this happens again, try clearing your cache."
		class = "toast-error"
	case "feedbackself":
		html = "You can't give yourself feedback."
		class = "toast-error"
	case "successvote":
		html = "Vote successful."
		class = "toast-success"
	case "successdelvote":
		html = "Vote successfully removed."
		class = "toast-success"
	case "successdel":
		html = "Successfully deleted."
		class = "toast-success"
	case "successadd":
		html = "Successfully added."
		class = "toast-success"
	case "successupdate":
		html = "Successfully updated."
		class = "toast-success"
	case "successaddfeedback":
		html = "Successfully added feedback."
		class = "toast-success"
	case "invalid":
		html = "Your SoundCloud url format is invalid."
		class = "toast-error"
	}

	if html != "" {
		return [2]string{html, class}
	}

	return [2]string{}
}
