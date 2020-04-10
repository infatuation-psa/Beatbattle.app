package main

import (
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
TODO
ICON, TITLE FOR EACH PAGE TO BE DYNAMIC
SUBMIT -> UPDATE when entry exists
MOVE SOUNDCLOUD/URL PROCESSING WORKLOAD TO THE TEMPLATE WITH ZINGGRID (URL AS REFERENCE)
---------*/

/*-------
Variables
-------*/

// store will hold all session data
var store *sessions.FilesystemStore
var redditAuth *reddit.Authenticator

// tmpl holds all parsed templates
var tmpl *template.Template
var policy *bluemonday.Policy

/*-------
Help Actions
--------*/

func init() {
	// loads values from .env into the system
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}

	policy = bluemonday.StrictPolicy()
	policy.AllowStandardURLs()

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
	router := pat.New()
	static := http.StripPrefix("/static/", http.FileServer(http.Dir("./static/")))

	state := string(RandString(16))
	redditAuth = reddit.NewAuthenticator(os.Getenv("REDDIT_KEY"), os.Getenv("REDDIT_SECRET"), os.Getenv("REDDIT_CALLBACK"),
		"linux:beatbattle:v0.1 (by /u/infatuationpsa)", state, reddit.ScopeIdentity)

	gothic.Store = sessions.NewCookieStore([]byte(os.Getenv("DISCORD_SECRET")))
	goth.UseProviders(discord.New(os.Getenv("DISCORD_KEY"), os.Getenv("DISCORD_SECRET"), os.Getenv("CALLBACK_URL"), discord.ScopeIdentify))

	router.PathPrefix("/static/").Handler(static)

	// Handlers for users & auth
	router.Get("/auth/{provider}/callback", Callback)
	router.Get("/auth/{provider}", Auth)
	router.Get("/logout/{provider}", Logout)
	router.Get("/logout", GenericLogout)
	router.Post("/vote/{id}", AddVote)
	router.Get("/login", Login)

	// Battle
	router.Post("/battle/{id}/update", UpdateBattleDB)
	router.Get("/battle/{id}/update", UpdateBattle)
	router.Get("/battle/{id}/delete", DeleteBattle)
	router.Post("/battle/submit", InsertBattle)
	router.Get("/battle/submit", SubmitBattle)
	router.Get("/battle/{id}", BattleHTTP)

	// Beat
	router.Get("/beat/{id}/submit", SubmitBeat)
	router.Post("/beat/{id}/submit", InsertBeat)
	router.Get("/beat/{id}/update", SubmitBeat)
	router.Get("/beat/{id}/delete", DeleteBeat)

	router.Get("/past", ViewBattles)
	router.Get("/", ViewBattles)

	http.Handle("/", router)

	if os.Getenv("PORT") == ":443" {
		log.Fatal(http.ListenAndServeTLS(os.Getenv("PORT"), "server.cert", "server.key", router))
	} else {
		log.Fatal(http.ListenAndServe(os.Getenv("PORT"), router))
	}
}
