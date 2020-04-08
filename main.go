package main

import (
	"encoding/gob"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

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

/*------
Structs
------*/

// Battle ...
type Battle struct {
	Title          string    `gorm:"column:title" json:"title"`
	Rules          string    `gorm:"column:rules" json:"rules"`
	Deadline       time.Time `gorm:"column:deadline" json:"deadline"`
	VotingDeadline time.Time `gorm:"column:voting_deadline" json:"voting_deadline"`
	Attachment     string    `gorm:"column:attachment" json:"attachment"`
	Host           string    `gorm:"column:host" json:"host"`
	Status         string    `gorm:"column:status" json:"status"`
	Password       string    `gorm:"column:password" json:"password"`
	UserID         string    `gorm:"column:user_id" json:"user_id"`
	Entries        int       `json:"entries"`
	ChallengeID    int       `gorm:"column:challenge_id" json:"challenge_id"`
	MaxVotes       int       `gorm:"column:maxvotes" json:"maxvotes"`
}

/*-------
Variables
-------*/

// store will hold all session data
var store *sessions.FilesystemStore

// tmpl holds all parsed templates
var tmpl *template.Template

/*-------
Help Actions
--------*/

func init() {
	// loads values from .env into the system
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}

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

func main() {
	gothic.Store = sessions.NewCookieStore([]byte(os.Getenv("DISCORD_SECRET")))
	goth.UseProviders(discord.New(os.Getenv("DISCORD_KEY"), os.Getenv("DISCORD_SECRET"), os.Getenv("CALLBACK_URL"), discord.ScopeIdentify, discord.ScopeEmail))

	router := pat.New()
	static := http.StripPrefix("/static/", http.FileServer(http.Dir("./static/")))

	router.PathPrefix("/static/").Handler(static)
	router.Get("/auth/{provider}/callback", Callback)
	router.Get("/auth/{provider}", Auth)
	router.Get("/logout/{provider}", Logout)
	router.Get("/submit/beat/{id}", SubmitBeat)
	router.Post("/submit/beat/{id}", InsertBeat)
	router.Get("/update/beat/{id}", SubmitBeat)
	router.Get("/battle/{id}", ViewBattle)
	router.Post("/vote/{id}", AddVote)
	router.Get("/submit/battle", SubmitBattle)
	router.Get("/", ViewBattles)

	http.Handle("/", router)

	if os.Getenv("PORT") == ":443" {
		log.Fatal(http.ListenAndServeTLS(os.Getenv("PORT"), "server.cert", "server.key", router))
	} else {
		log.Fatal(http.ListenAndServe(os.Getenv("PORT"), router))
	}
}
