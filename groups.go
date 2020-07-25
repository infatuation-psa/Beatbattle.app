package main

import (
	"database/sql"
	"encoding/json"
	"html"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/labstack/echo/v4"
)

// Group ...
type Group struct {
	ID              int           `gorm:"column:id" json:"id"`
	Title           string        `gorm:"column:title" json:"title" validate:"required"`
	Description     string        `gorm:"column:description" validate:"required"`
	DescriptionHTML template.HTML `json:"description"`
	Status          string        `gorm:"column:status"`
	StatusDisplay   string        `json:"status"`
	Owner           User
	Users           []GroupUser
}

// GroupUser ...
type GroupUser struct {
	ID       int    `gorm:"column:id" json:"id"`
	Nickname string `gorm:"column:nickname" json:"nickname" validate:"required"`
	Role     string `gorm:"column:role" json:"role"`
}

// SubmitGroup ...
func SubmitGroup(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	toast := GetToast(c)
	me := GetUser(c, false)
	ads := GetAdvertisements()

	m := map[string]interface{}{
		"Title": "Submit Group",
		"Me":    me,
		"Toast": toast,
		"Ads":   ads,
	}

	return c.Render(http.StatusOK, "SubmitGroup", m)
}

// ViewPublicGroups - Retrieves all groups and displays to user.
func ViewPublicGroups(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true

	me := GetUser(c, false)
	toast := GetToast(c)
	ads := GetAdvertisements()

	groups := GetGroups(db, 0)
	groupsJSON, _ := json.Marshal(groups)

	m := map[string]interface{}{
		"Title":  "Groups",
		"Groups": string(groupsJSON),
		"Me":     me,
		"Toast":  toast,
		"Ads":    ads,
	}

	return c.Render(http.StatusOK, "ViewPublicGroups", m)
}

// InsertGroup ...
func InsertGroup(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)

	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	title := policy.Sanitize(c.FormValue("title"))
	description := policy.Sanitize(c.FormValue("description"))
	inviteonly := policy.Sanitize(c.FormValue("inviteonly"))
	status := "open"

	println(inviteonly)
	if inviteonly == "on" {
		status = "inviteonly"
	}

	stmt := "INSERT INTO beatbattle.groups(title, description, status, owner_id) VALUES(?,?,?,?)"

	ins, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer ins.Close()

	insert, err := ins.Exec(title, description, status, me.ID)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	groupID, err := insert.LastInsertId()
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	stmt = "INSERT INTO users_groups(user_id, group_id, role) VALUES(?,?,?)"

	ins, err = db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer ins.Close()

	ins.Exec(me.ID, groupID, "owner")

	SetToast(c, "successadd")
	return c.Redirect(302, "/group/"+strconv.Itoa(int(groupID)))
}

// InsertGroupInvite ...
func InsertGroupInvite(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is authenticated.
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "login")
	}

	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	groupID, err := strconv.Atoi(policy.Sanitize(c.FormValue("group")))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	inviteExists := RowExists("SELECT id FROM groups_invites WHERE user_id = ? AND group_id = ?", userID, groupID)
	if inviteExists {
		SetToast(c, "invexists")
		return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
	}

	hasPermissions := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ? AND role = ?", me.ID, groupID, "owner")
	if !hasPermissions {
		SetToast(c, "notuser")
		return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
	}

	stmt := "INSERT INTO groups_invites(user_id, group_id) VALUES(?,?)"
	ins, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
	}
	defer ins.Close()

	ins.Exec(userID, groupID)

	SetToast(c, "successinv")
	return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
}

// InsertGroupRequest ...
func InsertGroupRequest(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	userInGroup := RowExists("SELECT id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, groupID)
	if userInGroup {
		SetToast(c, "ingroup")
		return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
	}

	requestExists := RowExists("SELECT id FROM groups_requests WHERE user_id = ? AND group_id = ?", me.ID, groupID)
	if requestExists {
		SetToast(c, "reqexists")
		return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
	}

	hasPermissions := RowExists("SELECT id FROM beatbattle.groups WHERE id = ? and status=?", groupID, "open")
	if !hasPermissions {
		SetToast(c, "notopengrp")
		return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
	}

	stmt := "INSERT INTO groups_requests(user_id, group_id) VALUES(?,?)"

	ins, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
	}
	defer ins.Close()

	ins.Exec(me.ID, groupID)

	SetToast(c, "successreq")
	return c.Redirect(302, "/group/"+strconv.Itoa(groupID))
}

// GroupInviteResponse ...
func GroupInviteResponse(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	// Check if user is properly authenticated before modifying DB.
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	// Get the group invite from context.
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/me/groups")
	}

	inviteExists := RowExists("SELECT user_id FROM groups_invites WHERE user_id = ? AND group_id = ?", me.ID, groupID)
	if !inviteExists {
		SetToast(c, "404")
		return c.Redirect(302, "/me/groups")
	}

	response := c.Param("response")
	if response != "accept" && response != "decline" {
		SetToast(c, "502")
		return c.Redirect(302, "/me/groups")
	}

	if response == "accept" {
		inGroup := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, groupID)
		if inGroup {
			response = "ingroup"
		}

		if !inGroup {
			stmt := "INSERT INTO users_groups(user_id, group_id, role) VALUES(?,?,'member')"
			ins, err := db.Prepare(stmt)
			if err != nil {
				SetToast(c, "502")
				return c.Redirect(302, "/me/groups")
			}
			defer ins.Close()

			ins.Exec(me.ID, groupID)
		}
	}

	stmt := "DELETE FROM groups_invites WHERE user_id = ? and group_id = ?"
	del, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/me/groups")
	}
	defer del.Close()
	del.Exec(me.ID, groupID)

	SetToast(c, response)
	return c.Redirect(302, "/me/groups")
}

// GroupRequestResponse ...
func GroupRequestResponse(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	requestID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/me/groups")
	}

	userID, groupID := 0, 0
	err = db.QueryRow("SELECT user_id, group_id FROM groups_requests WHERE id = ?", requestID).Scan(&userID, &groupID)
	if err != nil {
		SetToast(c, "404")
		return c.Redirect(302, "/me/groups")
	}

	response := c.Param("response")
	if response != "accept" && response != "decline" {
		SetToast(c, "502")
		return c.Redirect(302, "/me/groups")
	}

	if response == "accept" {
		inGroup := RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", userID, groupID)
		if inGroup {
			response = "ingroup"
		}

		if !inGroup {
			stmt := "INSERT INTO users_groups(user_id, group_id, role) VALUES(?,?,'member')"
			ins, err := db.Prepare(stmt)
			if err != nil {
				SetToast(c, "502")
				return c.Redirect(302, "/me/groups")
			}
			defer ins.Close()
			ins.Exec(userID, groupID)
		}
	}

	stmt := "DELETE FROM groups_requests WHERE id = ?"
	del, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/me/groups")
	}
	defer del.Close()
	del.Exec(requestID)

	SetToast(c, response+"req")
	return c.Redirect(302, "/me/groups")
}

// GetUserGroups ...
func GetUserGroups(db *sql.DB, value int) ([]Group, []Group, []Group) {
	queryRequests := `SELECT groups_requests.group_id, groups.title, groups.owner_id
					FROM groups_requests
					INNER JOIN groups ON groups_requests.group_id = groups.id
					INNER JOIN users_groups ON users_groups.group_id = groups_requests.group_id
					WHERE users_groups.user_id = ? AND users_groups.role = "owner"`
	queryInvites := `SELECT groups.id, groups.title, groups.description, "invited", groups.owner_id
					FROM groups_invites
					LEFT JOIN beatbattle.groups ON groups.id = groups_invites.group_id 
					WHERE groups_invites.user_id = ?`
	queryGroups := `SELECT groups.id, groups.title, groups.description, users_groups.role, groups.owner_id
					FROM users_groups
					LEFT JOIN beatbattle.groups ON groups.id = users_groups.group_id 
					WHERE users_groups.user_id = ?`

	requests := []Group{}
	invites := []Group{}
	groups := []Group{}

	// Get the request rows from DB.
	requestsRows, err := db.Query(queryRequests, value)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, nil
	}
	defer requestsRows.Close()

	for requestsRows.Next() {
		group := Group{}
		err = requestsRows.Scan(&group.ID, &group.Title, &group.Owner.ID)
		if err != nil {
			return nil, nil, nil
		}
		group.Owner = GetUserDB(group.Owner.ID)
		group.StatusDisplay = "Requested"
		requests = append(requests, group)
	}

	if err = requestsRows.Err(); err != nil {
		log.Println(err)
	}
	if err = requestsRows.Close(); err != nil {
		log.Println(err)
	}

	// Get the invite rows from DB.
	invitesRows, err := db.Query(queryInvites, value)
	if err != nil && err != sql.ErrNoRows {
		return requests, nil, nil
	}
	defer invitesRows.Close()

	for invitesRows.Next() {
		group := Group{}
		err = invitesRows.Scan(&group.ID, &group.Title, &group.Description, &group.Status, &group.Owner.ID)
		if err != nil {
			return requests, nil, nil
		}
		group.Owner = GetUserDB(group.Owner.ID)
		group.StatusDisplay = "Invited"
		invites = append(invites, group)
	}

	if err = invitesRows.Err(); err != nil {
		log.Println(err)
	}
	if err = invitesRows.Close(); err != nil {
		log.Println(err)
	}

	// Get the group rows from DB.
	groupsRows, err := db.Query(queryGroups, value)
	if err != nil && err != sql.ErrNoRows {
		return requests, invites, nil
	}
	defer groupsRows.Close()

	for groupsRows.Next() {
		group := Group{}
		err = groupsRows.Scan(&group.ID, &group.Title, &group.Description, &group.Status, &group.Owner.ID)
		if err != nil {
			return requests, invites, nil
		}
		group.Owner = GetUserDB(group.Owner.ID)
		group.StatusDisplay = strings.Title(group.Status)
		groups = append(groups, group)
	}

	if err = groupsRows.Err(); err != nil {
		log.Println(err)
	}
	if err = groupsRows.Close(); err != nil {
		log.Println(err)
	}

	return requests, invites, groups
}

// GetGroupsByRole ...
func GetGroupsByRole(db *sql.DB, value int, role string) []Group {
	query := `SELECT group_info.id, group_info.title from users_groups
			LEFT JOIN beatbattle.groups AS group_info ON group_info.id = users_groups.group_id
			WHERE users_groups.user_id = ? and users_groups.role = ?`

	args := []interface{}{value, role}

	if role == "member" {
		query = `SELECT group_info.id, group_info.title from users_groups
				LEFT JOIN beatbattle.groups AS group_info ON group_info.id = users_groups.group_id
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
		err = rows.Scan(&group.ID, &group.Title)
		if err != nil {
			return nil
		}

		groups = append(groups, group)
	}
	if err = rows.Err(); err != nil {
		// handle the error here
	}
	if err = rows.Close(); err != nil {
		// but what should we do if there's an error?
		log.Println(err)
	}

	return groups
}

// GetGroups retrieves groups from the database using a field and value.
func GetGroups(db *sql.DB, value int) []Group {
	query := `SELECT groups.id, groups.title, groups.description, groups.status, groups.owner_id
			FROM beatbattle.groups`
	args := []interface{}{}

	if value > 0 {
		query = `SELECT groups.id, groups.title, groups.description, users_groups.role, groups.owner_id
				FROM users_groups
				LEFT JOIN beatbattle.groups ON groups.id = users_groups.group_id 
				WHERE users_groups.user_id = ?`
		args = []interface{}{value}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	groups := []Group{}

	for rows.Next() {
		group := Group{}
		err = rows.Scan(&group.ID, &group.Title, &group.Description, &group.Status, &group.Owner.ID)
		if err != nil {
			return nil
		}

		group.Owner = GetUserDB(group.Owner.ID)

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

	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	return groups
}

// GetGroup retrieves a group from the database using an ID.
func GetGroup(db *sql.DB, groupID int) Group {
	users := []GroupUser{}
	group := Group{}

	query := `SELECT groups.id, groups.title, groups.description, groups.status, groups.owner_id
			FROM beatbattle.groups  
			WHERE groups.id = ?`

	err := db.QueryRow(query, groupID).Scan(&group.ID, &group.Title, &group.Description,
		&group.Status, &group.Owner.ID)

	if err != nil {
		return group
	}

	group.Title = html.UnescapeString(group.Title)
	group.Owner = GetUserDB(group.Owner.ID)

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
func GroupHTTP(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	toast := GetToast(c)
	ads := GetAdvertisements()
	isOwner, inGroup, invited, requested := false, false, false, false

	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	// Retrieve group, return to front page if group doesn't exist.
	group := GetGroup(db, groupID)
	if group.Users == nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	e, err := json.Marshal(group.Users)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}

	me := GetUser(c, false)
	if me.Authenticated {
		isOwner = group.Owner.ID == me.ID
		inGroup = RowExists("SELECT user_id FROM users_groups WHERE user_id = ? AND group_id = ?", me.ID, groupID)
		invited = RowExists("SELECT user_id FROM groups_invites WHERE user_id = ? AND group_id = ?", me.ID, groupID)
		requested = RowExists("SELECT user_id FROM groups_requests WHERE user_id = ? AND group_id = ?", me.ID, groupID)
	}

	m := map[string]interface{}{
		"Title":     group.Title,
		"Group":     group,
		"Users":     string(e),
		"Me":        me,
		"IsOwner":   isOwner,
		"InGroup":   inGroup,
		"Invited":   invited,
		"Requested": requested,
		"Toast":     toast,
		"Ads":       ads,
	}

	return c.Render(http.StatusOK, "Group", m)
}

// UpdateGroup ...
func UpdateGroup(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	toast := GetToast(c)
	ads := GetAdvertisements()

	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	me := GetUser(c, false)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	isOwner := RowExists("SELECT id FROM beatbattle.groups WHERE owner_id = ? AND id = ?", me.ID, groupID)
	if !isOwner {
		SetToast(c, "notuser")
		return c.Redirect(302, "/")
	}

	// Retrieve group, return to front page if group doesn't exist.
	group := GetGroup(db, groupID)

	if group.Users == nil {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	inviteOnly := false
	if group.Status == "inviteonly" {
		inviteOnly = true
	}

	m := map[string]interface{}{
		"Title":      group.Title,
		"Group":      group,
		"Me":         me,
		"Toast":      toast,
		"InviteOnly": inviteOnly,
		"Ads":        ads,
	}

	return c.Render(http.StatusOK, "UpdateGroup", m)
}

// UpdateGroupDB ...
func UpdateGroupDB(c echo.Context) error {
	// Set the request to close automatically.
	c.Request().Header.Set("Connection", "close")
	c.Request().Close = true
	me := GetUser(c, true)
	if !me.Authenticated {
		SetToast(c, "relog")
		return c.Redirect(302, "/login")
	}

	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil && err != sql.ErrNoRows {
		SetToast(c, "404")
		return c.Redirect(302, "/")
	}

	isOwner := RowExists("SELECT id FROM beatbattle.groups WHERE owner_id = ? AND id = ?", me.ID, groupID)
	if !isOwner {
		SetToast(c, "notuser")
		return c.Redirect(302, "/")
	}

	status := "open"
	title := policy.Sanitize(c.FormValue("title"))
	description := policy.Sanitize(c.FormValue("description"))
	inviteonly := policy.Sanitize(c.FormValue("inviteonly"))
	if inviteonly == "on" {
		status = "inviteonly"
	}

	stmt := "UPDATE beatbattle.groups SET title = ?, description = ?, status = ? WHERE id = ?"
	upd, err := db.Prepare(stmt)
	if err != nil {
		SetToast(c, "502")
		return c.Redirect(302, "/")
	}
	defer upd.Close()
	upd.Exec(title, description, status, groupID)

	SetToast(c, "successupdate")
	return c.Redirect(302, "/group/"+strconv.Itoa(int(groupID)))
}
