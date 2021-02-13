package main

import (
	"database/sql"
	"encoding/gob"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/cameronstanley/go-reddit"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/discord"
	"github.com/markbates/goth/providers/twitch"
	"github.com/microcosm-cc/bluemonday"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

/*-------
Variables
-------*/

// store will hold all session data
var store *sessions.CookieStore
var discordProvider *discord.Provider
var twitchProvider *twitch.Provider
var redditAuth *reddit.Authenticator

var policy *bluemonday.Policy

var whitelist []string
var state string
var analyticsKey string
var dbWrite, dbRead *sql.DB
var e *echo.Echo

/*-------
Help Actions
--------*/

func init() {
	// TODO - BREAK WHITELIST UP INTO TWO (TRACK WHITELIST, ATTACHMENT WHITELIST)
	whitelist = []string{"audius.co", "archive.org", "f1eightco-my.sharepoint.com", "sharepoint.com", "drive.google.com", "youtube.com", "bandcamp.com", "soundcloud.com", "sellfy.com", "onedrive.com", "dropbox.com", "mega.nz", "amazon.com/clouddrive", "filetransfer.io", "wetransfer.com", "we.tt"}
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

	policy = bluemonday.UGCPolicy()
	//policy.AllowStandardURLs()

	// Session
	authKeyOne := []byte(os.Getenv("SECURE_KEY64"))
	encryptionKeyOne := []byte(os.Getenv("SECURE_KEY32"))
	analyticsKey = os.Getenv("ANALYTICS_ID")

	store = sessions.NewCookieStore(authKeyOne, encryptionKeyOne)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   60 * 10080,
		HttpOnly: true,
	}

	gob.Register(User{})

	dbRead, dbWrite = dbInit()

	e = echo.New()

	e.Server.WriteTimeout = 10 * time.Second
	e.Server.ReadTimeout = 5 * time.Second
	e.Server.IdleTimeout = 10 * time.Second

	// Enable metrics middleware
	// p := prometheus.NewPrometheus("echo", nil)
	// p.Use(e)

	e.Pre(middleware.HTTPSNonWWWRedirect())
	e.Use(middleware.Secure())
	e.Pre(middleware.RemoveTrailingSlash())

	//e.Use(middleware.Logger())

	tmpl := &Template{
		templates: template.Must(template.New("base").Funcs(sprig.FuncMap()).ParseGlob("templates/*.tmpl")),
	}

	e.Renderer = tmpl
	e.Static("/static", "static")
}

// Template struct
type Template struct {
	templates *template.Template
}

// Advertisement struct
type Advertisement struct {
	ID    int    `gorm:"column:id" json:"id"`
	URL   string `gorm:"column:url" json:"url"`
	Image string `gorm:"column:image" json:"image"`
}

// Render func
func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

const charset = "abcdefghijklmnopqrstuvwxyz" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

// StringWithCharset ...
func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// FrequentQuestions ...
func FrequentQuestions(c echo.Context) error {
	me := GetUser(c, false)
	toast := GetToast(c)
	ads := GetAdvertisements()

	m := map[string]interface{}{
		"Meta": map[string]interface{}{
			"Title":   "Frequently Asked Questions",
			"Analytics":   analyticsKey,
		},
		"Me":    me,
		"Toast": toast,
		"Ads":   ads,
	}

	return c.Render(http.StatusOK, "FAQ", m)
}

// RandString ...
func RandString(length int) string {
	return StringWithCharset(length, charset)
}

func main() {
	defer dbWrite.Close()
	defer dbRead.Close()
	// TODO - IS IT SAFE TO STORE STATE?
	state = os.Getenv("REDDIT_STATE")

	redditAuth = reddit.NewAuthenticator(os.Getenv("REDDIT_KEY"), os.Getenv("REDDIT_SECRET"), os.Getenv("REDDIT_CALLBACK"),
		"linux:beatbattle:v1.3 (by /u/infatuationpsa)", state, reddit.ScopeIdentity)
	redditAuth.RequestPermanentToken = true

	// TODO DEPRECATE GOTHIC/GOTH
	gothic.Store = store //sessions.NewCookieStore([]byte(os.Getenv("DISCORD_SECRET")))

	discordProvider = discord.New(os.Getenv("DISCORD_KEY"), os.Getenv("DISCORD_SECRET"), os.Getenv("DISCORD_CALLBACK"), discord.ScopeIdentify, discord.ScopeGuilds)
	twitchProvider = twitch.New(os.Getenv("TWITCH_KEY"), os.Getenv("TWITCH_SECRET"), os.Getenv("TWITCH_CALLBACK"), twitch.ScopeChannelCheckSubscription)

	goth.UseProviders(discordProvider)
	goth.UseProviders(twitchProvider)

	// Handlers for users & auth
	e.GET("/auth/callback", Callback)
	e.GET("/auth", Auth)
	e.GET("/logout/:provider", Logout)
	e.GET("/logout", Logout)
	e.POST("/feedback", AddFeedback)
	e.POST("/like", AddLike)
	e.POST("/placement", SetPlacement)
	e.POST("/disqualify", DisqualifyBeat)
	e.POST("/vote", AddVote)
	e.GET("/login", Login)
	e.GET("/faq", FrequentQuestions)

	// Me
	e.GET("/user/:id/submissions", UserSubmissions)
	e.GET("/user/:id", UserBattles)
	
	// Battles
	e.GET("/battles/:tag", ViewTaggedBattles)

	// Battle
	e.GET("/battle/:id/update/timezone/:region/:country", UpdateBattle) // Timezone
	e.POST("/battle/:id/update", UpdateBattleDB)                        // Update in db
	e.GET("/battle/:id/update", UpdateBattle)                           // Update page
	e.POST("/battle/:id/delete", DeleteBattle)                  
	e.POST("/battle/:id/close", CloseBattle)
	e.GET("/battle/:id/feedback", ViewFeedback)

	e.POST("/battle/submit", InsertBattle)
	e.GET("/battle/submit", SubmitBattle)
	e.GET("/battle/:id", BattleHTTP)

	// Beat
	e.GET("/beat/:id/submit", SubmitBeat)
	e.POST("/beat/:id/submit", InsertBeat)
	e.POST("/beat/:id/update", UpdateBeat)
	e.GET("/beat/:id/update", SubmitBeat)
	e.GET("/beat/:id/delete", DeleteBeat)

	e.GET("/past", ViewBattles)
	e.GET("/", ViewBattles)

	e.GET("/request", func(c echo.Context) error {
		req := c.Request()
		format := `
		  <code>
			Protocol: %s<br>
			Host: %s<br>
			Remote Address: %s<br>
			Method: %s<br>
			Path: %s<br>
		  </code>
		`
		return c.HTML(http.StatusOK, fmt.Sprintf(format, req.Proto, req.Host, req.RemoteAddr, req.Method, req.URL.Path))
	})
	
	// We should try to make the alive check only respond to http to unrequire this.
	go ListenHTTP()

	e.Logger.Fatal(e.StartTLS(":443", "server.crt", "server.key"))
}

// ListenHTTP starts the HTTP server
func ListenHTTP() {
	e.Logger.Fatal(e.Start(":5000"))
}

// ContainsString just checks if the str is whthin the array.
func ContainsString(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

// ContainsInt just checks if the int is whthin the array.
func ContainsInt(arr []int, integer int) bool {
	for _, a := range arr {
		if a == integer {
			return true
		}
	}
	return false
}
