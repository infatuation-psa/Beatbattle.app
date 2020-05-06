package main

import "net/http"

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
