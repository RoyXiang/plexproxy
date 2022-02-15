package handler

type PlexAccount struct {
	User struct {
		Id int64 `json:"id"`
	} `json:"user"`
}
