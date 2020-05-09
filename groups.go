package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gomarkdown/markdown"
)

// Group ...
type Group struct {
	Title           string        `gorm:"column:title" json:"title" validate:"required"`
	Description     string        `gorm:"column:description" validate:"required"`
	DescriptionHTML template.HTML `json:"description"`
	Status          string        `gorm:"column:status"`
	StatusDisplay   string        `json:"status"`
	ID              int           `gorm:"column:id" json:"id"`
	OwnerID         int           `gorm:"column:owner_id" json:"owner_id"`
	OwnerNickname   string        `gorm:"column:owner_nickname" json:"owner_nickname"`
	Users           []GroupUser
}

// GroupUser ...
type GroupUser struct {
	Nickname string `gorm:"column:nickname" json:"nickname" validate:"required"`
	Role     string `gorm:"column:role" json:"role"`
	ID       int    `gorm:"column:id" json:"id"`
}

// SubmitGroup ...
func SubmitGroup(w http.ResponseWriter, r *http.Request) {
	toast := GetToast(r.URL.Query().Get(":toast"))
	defer r.Body.Close()

	var user = GetUser(w, r)
	m := map[string]interface{}{
		"Title": "Submit Group",
		"User":  user,
		"Toast": toast,
	}

	tmpl.ExecuteTemplate(w, "SubmitGroup", m)
}

// ViewGroups - Retrieves all groups and displays to user.
func ViewGroups(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(r.URL.Query().Get(":toast"))

	user := GetUser(w, r)

	groups := GetGroups(db, 0)

	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		fmt.Println(err)
		return
	}

	m := map[string]interface{}{
		"Title":  "Groups",
		"Groups": string(groupsJSON),
		"User":   user,
		"Toast":  toast,
	}

	tmpl.ExecuteTemplate(w, "ViewGroups", m)
}

// InsertGroup ...
func InsertGroup(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}
	defer r.Body.Close()

	title := policy.Sanitize(r.FormValue("title"))
	description := policy.Sanitize(r.FormValue("description"))
	inviteonly := policy.Sanitize(r.FormValue("inviteonly"))
	status := "open"

	println(inviteonly)
	if inviteonly == "on" {
		status = "inviteonly"
	}

	stmt := "INSERT INTO beatbattle.groups(title, description, status, owner_id) VALUES(?,?,?,?)"

	ins, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer ins.Close()

	insert, err := ins.Exec(title, description, status, user.ID)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}

	groupID, err := insert.LastInsertId()
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}

	stmt = "INSERT INTO users_groups(user_id, group_id, role) VALUES(?,?,?)"

	ins, err = db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer ins.Close()

	ins.Exec(user.ID, groupID, "owner")

	http.Redirect(w, r, "/group/"+strconv.Itoa(int(groupID))+"/successadd", 302)
	return
}

// InsertGroupInvite ...
func InsertGroupInvite(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}
	defer r.Body.Close()

	userID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	groupID, err := strconv.Atoi(policy.Sanitize(r.FormValue("group")))
	if err != nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	inviteExists := RowExists(db, "SELECT id FROM groups_invites WHERE user_id = ? AND group_id = ?", userID, groupID)

	if inviteExists {
		http.Redirect(w, r, "/group/"+strconv.Itoa(groupID)+"/invexists", 302)
		return
	}

	hasPermissions := RowExists(db, "SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ? AND role = ?", user.ID, groupID, "owner")

	if !hasPermissions {
		http.Redirect(w, r, "/group/"+strconv.Itoa(groupID)+"/notuser", 302)
		return
	}

	stmt := "INSERT INTO groups_invites(user_id, group_id) VALUES(?,?)"

	ins, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, "/group/"+strconv.Itoa(groupID)+"/502", 302)
		return
	}
	defer ins.Close()

	ins.Exec(userID, groupID)

	http.Redirect(w, r, "/group/"+strconv.Itoa(groupID)+"/successinv", 302)
	return
}

// GroupInviteResponse ...
func GroupInviteResponse(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}
	defer r.Body.Close()

	groupID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/me/groups/404", 302)
		return
	}

	inviteExists := RowExists(db, "SELECT user_id FROM groups_invites WHERE user_id = ? AND group_id = ?", user.ID, groupID)
	if !inviteExists {
		http.Redirect(w, r, "/me/groups/404", 302)
		return
	}

	response := r.URL.Query().Get(":response")

	if response != "accept" && response != "decline" {
		http.Redirect(w, r, "/me/groups/502", 302)
		return
	}

	if response == "accept" {
		inGroup := RowExists(db, "SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", user.ID, groupID)
		if inGroup {
			response = "ingroup"
		}

		if !inGroup {
			stmt := "INSERT INTO users_groups(user_id, group_id, role) VALUES(?,?,'member')"

			ins, err := db.Prepare(stmt)
			if err != nil {
				http.Redirect(w, r, "/me/groups/502", 302)
				return
			}
			defer ins.Close()

			ins.Exec(user.ID, groupID)
		}
	}

	stmt := "DELETE FROM groups_invites WHERE user_id = ? and group_id = ?"

	del, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, "/me/groups/502", 302)
		return
	}
	defer del.Close()

	del.Exec(user.ID, groupID)

	http.Redirect(w, r, "/me/groups/"+response, 302)
	return
}

// GroupRequestResponse ...
func GroupRequestResponse(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}
	defer r.Body.Close()

	requestID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/me/groups/404", 302)
		return
	}

	userID := 0
	groupID := 0
	err = db.QueryRow("SELECT user_id, group_id FROM groups_requests WHERE id = ?", requestID).Scan(&userID, &groupID)
	if err != nil {
		http.Redirect(w, r, "/me/groups/404", 302)
		return
	}

	response := r.URL.Query().Get(":response")

	if response != "accept" && response != "decline" {
		http.Redirect(w, r, "/me/groups/502", 302)
		return
	}

	if response == "accept" {
		inGroup := RowExists(db, "SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", userID, groupID)
		if inGroup {
			response = "ingroup"
		}

		if !inGroup {
			stmt := "INSERT INTO users_groups(user_id, group_id, role) VALUES(?,?,'member')"

			ins, err := db.Prepare(stmt)
			if err != nil {
				http.Redirect(w, r, "/me/groups/502", 302)
				return
			}
			defer ins.Close()

			ins.Exec(user.ID, groupID)
		}
	}

	stmt := "DELETE FROM groups_requests WHERE id = ?"

	del, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, "/me/groups/502", 302)
		return
	}
	defer del.Close()

	del.Exec(requestID)

	http.Redirect(w, r, "/me/groups/"+response+"req", 302)
	return
}

// GetUserGroups ...
func GetUserGroups(db *sql.DB, value int) ([]Group, []Group, []Group) {
	queryRequests := `SELECT groups.id as group_id, groups.title, groups_requests.id as request_id, nickname from groups_requests
					LEFT JOIN users_groups on groups_requests.group_id = users_groups.group_id
					LEFT JOIN users on groups_requests.user_id = users.id
					LEFT JOIN beatbattle.groups on groups.id = groups_requests.group_id
					WHERE users_groups.user_id = ? and users_groups.role = "owner"`

	queryInvites := `SELECT groups.id, groups.title, groups.description, "invited", groups.owner_id, users.nickname
					FROM groups_invites
					LEFT JOIN beatbattle.groups ON groups.id = groups_invites.group_id 
					LEFT JOIN users on users.id=groups.owner_id
					WHERE groups_invites.user_id = ?`

	queryGroups := `SELECT groups.id, groups.title, groups.description, "requested", groups.owner_id, users.nickname
					FROM groups_requests
					LEFT JOIN beatbattle.groups ON groups.id = groups_requests.group_id 
					LEFT JOIN users on users.id=groups.owner_id
					WHERE groups_requests.user_id = ?
					UNION
					SELECT groups.id, groups.title, groups.description, users_groups.role, groups.owner_id, users.nickname
					FROM users_groups
					LEFT JOIN beatbattle.groups ON groups.id = users_groups.group_id 
					LEFT JOIN users on users.id=groups.owner_id
					WHERE users_groups.user_id = ?`

	requests := []Group{}
	invites := []Group{}
	groups := []Group{}

	rows, err := db.Query(queryRequests, value)

	if err != nil && err != sql.ErrNoRows {
		return nil, nil, nil
	}
	defer rows.Close()

	for rows.Next() {
		group := Group{}
		err = rows.Scan(&group.ID, &group.Title, &group.OwnerID, &group.OwnerNickname)
		if err != nil {
			return nil, nil, nil
		}

		group.StatusDisplay = "Requested"

		requests = append(requests, group)
	}

	rows, err = db.Query(queryInvites, value)

	if err != nil && err != sql.ErrNoRows {
		return requests, nil, nil
	}
	defer rows.Close()

	for rows.Next() {
		group := Group{}
		err = rows.Scan(&group.ID, &group.Title, &group.Description, &group.Status, &group.OwnerID, &group.OwnerNickname)
		if err != nil {
			return requests, nil, nil
		}

		group.StatusDisplay = "Invited"

		invites = append(invites, group)
	}

	rows, err = db.Query(queryGroups, value, value)

	if err != nil && err != sql.ErrNoRows {
		return requests, invites, nil
	}
	defer rows.Close()

	for rows.Next() {
		group := Group{}
		err = rows.Scan(&group.ID, &group.Title, &group.Description, &group.Status, &group.OwnerID, &group.OwnerNickname)
		if err != nil {
			return requests, invites, nil
		}

		group.StatusDisplay = strings.Title(group.Status)

		groups = append(groups, group)
	}

	return requests, invites, groups
}

// GetGroupsByRole ...
func GetGroupsByRole(db *sql.DB, value int, role string) []Group {
	query := `SELECT group_info.id, group_info.title from users_groups
			LEFT JOIN beatbattle.groups AS group_info ON group_info.id = users_groups.group_id
			WHERE users_groups.user_id = ? and users_groups.role = ?`

	rows, err := db.Query(query, value, role)

	if err != nil {
		return nil
	}
	defer rows.Close()

	group := Group{}
	groups := []Group{}

	for rows.Next() {
		err = rows.Scan(&group.ID, &group.Title)
		if err != nil {
			return nil
		}

		groups = append(groups, group)
	}

	return groups
}

// GetGroups retrieves battles from the database using a field and value.
func GetGroups(db *sql.DB, value int) []Group {
	query := `SELECT groups.id, groups.title, groups.description, groups.status, groups.owner_id, users.nickname
			FROM beatbattle.groups
			LEFT JOIN users on users.id=groups.owner_id`

	args := []interface{}{}

	if value > 0 {
		query = `SELECT groups.id, groups.title, groups.description, users_groups.role, groups.owner_id, users.nickname
				FROM users_groups
				LEFT JOIN beatbattle.groups ON groups.id = users_groups.group_id 
				LEFT JOIN users on users.id=groups.owner_id
				WHERE users_groups.user_id = ?`
		args = []interface{}{value}
	}

	rows, err := db.Query(query, args...)

	if err != nil {
		return nil
	}
	defer rows.Close()

	group := Group{}
	groups := []Group{}

	for rows.Next() {
		err = rows.Scan(&group.ID, &group.Title, &group.Description, &group.Status, &group.OwnerID, &group.OwnerNickname)
		if err != nil {
			return nil
		}

		switch group.Status {
		case "owner":
			group.StatusDisplay = "Owner"
		case "invited":
			group.StatusDisplay = "Invited"
		case "inviteonly":
			group.StatusDisplay = "Invite Only"
		case "open":
			group.StatusDisplay = "Open"
		default:
			group.StatusDisplay = "Requested"
		}

		groups = append(groups, group)
	}

	return groups
}

// GetGroup retrieves a group from the database using an ID.
func GetGroup(db *sql.DB, groupID int) Group {
	users := []GroupUser{}
	group := Group{}

	query := `
			SELECT groups.id, groups.title, groups.description, groups.status, groups.owner_id, users.nickname
			FROM beatbattle.groups 
			LEFT JOIN users ON groups.owner_id = users.id 
			WHERE groups.id = ?`

	err := db.QueryRow(query, groupID).Scan(&group.ID, &group.Title, &group.Description,
		&group.Status, &group.OwnerID, &group.OwnerNickname)

	if err != nil {
		return group
	}

	group.Title = html.UnescapeString(group.Title)
	group.OwnerNickname = html.UnescapeString(group.OwnerNickname)

	md := []byte(html.UnescapeString(group.Description))
	group.Description = html.UnescapeString(group.Description)
	group.DescriptionHTML = template.HTML(markdown.ToHTML(md, nil, nil))

	switch group.Status {
	case "inviteonly":
		group.StatusDisplay = "Invite Only"
	default:
		group.StatusDisplay = "Open"
	}

	groupUsers, err := db.Query("SELECT user_id, role, users.nickname FROM users_groups LEFT JOIN users on users.id = users_groups.user_id WHERE group_id = ?", groupID)
	if err != nil && err != sql.ErrNoRows {
		return group
	}

	defer groupUsers.Close()

	for groupUsers.Next() {
		user := GroupUser{}
		err = groupUsers.Scan(&user.ID, &user.Role, &user.Nickname)
		if err != nil {
			return group
		}

		user.Role = strings.Title(user.Role)
		users = append(users, user)
	}

	group.Users = users

	return group
}

// GroupHTTP - Retrieves group and displays to user.
func GroupHTTP(w http.ResponseWriter, r *http.Request) {
	toast := GetToast(r.URL.Query().Get(":toast"))
	defer r.Body.Close()

	db := dbConn()
	defer db.Close()

	isOwner := false
	inGroup := false
	invited := false
	requested := false

	groupID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}

	user := GetUser(w, r)

	// Retrieve group, return to front page if group doesn't exist.
	group := GetGroup(db, groupID)

	if group.Users == nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	e, err := json.Marshal(group.Users)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}

	if user.Authenticated {
		isOwner = RowExists(db, "SELECT id FROM beatbattle.groups WHERE owner_id = ? AND id = ?", user.ID, groupID)
		inGroup = RowExists(db, "SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", user.ID, groupID)
		invited = RowExists(db, "SELECT user_id FROM groups_invites WHERE user_id = ? AND group_id = ?", user.ID, groupID)
		requested = RowExists(db, "SELECT user_id FROM groups_requests WHERE user_id = ? AND group_id = ?", user.ID, groupID)
	}

	m := map[string]interface{}{
		"Title":     group.Title,
		"Group":     group,
		"Users":     string(e),
		"User":      user,
		"IsOwner":   isOwner,
		"InGroup":   inGroup,
		"Invited":   invited,
		"Requested": requested,
		"Toast":     toast,
	}

	tmpl.ExecuteTemplate(w, "Group", m)
}

// UpdateGroup ...
func UpdateGroup(w http.ResponseWriter, r *http.Request) {
	toast := GetToast(r.URL.Query().Get(":toast"))
	defer r.Body.Close()

	db := dbConn()
	defer db.Close()
	groupID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}

	user := GetUser(w, r)

	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}

	isOwner := RowExists(db, "SELECT id FROM beatbattle.groups WHERE owner_id = ? AND id = ?", user.ID, groupID)
	if !isOwner {
		http.Redirect(w, r, "/notuser", 302)
		return
	}

	// Retrieve group, return to front page if group doesn't exist.
	group := GetGroup(db, groupID)

	if group.Users == nil {
		http.Redirect(w, r, "/404", 302)
		return
	}

	inviteOnly := false
	if group.Status == "inviteonly" {
		inviteOnly = true
	}

	m := map[string]interface{}{
		"Title":      group.Title,
		"Group":      group,
		"User":       user,
		"Toast":      toast,
		"InviteOnly": inviteOnly,
	}

	tmpl.ExecuteTemplate(w, "UpdateGroup", m)
}

// UpdateGroupDB ...
func UpdateGroupDB(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 302)
		return
	}
	defer r.Body.Close()

	groupID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil && err != sql.ErrNoRows {
		http.Redirect(w, r, "/404", 302)
		return
	}

	isOwner := RowExists(db, "SELECT id FROM beatbattle.groups WHERE owner_id = ? AND id = ?", user.ID, groupID)
	if !isOwner {
		http.Redirect(w, r, "/notuser", 302)
		return
	}

	title := policy.Sanitize(r.FormValue("title"))
	description := policy.Sanitize(r.FormValue("description"))
	inviteonly := policy.Sanitize(r.FormValue("inviteonly"))
	status := "open"

	if inviteonly == "on" {
		status = "inviteonly"
	}

	stmt := "UPDATE beatbattle.groups SET title = ?, description = ?, status = ? WHERE id = ?"

	upd, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, "/502", 302)
		return
	}
	defer upd.Close()

	upd.Exec(title, description, status, groupID)

	http.Redirect(w, r, "/group/"+strconv.Itoa(int(groupID))+"/successupdate", 302)
	return
}
