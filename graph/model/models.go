package model

// Go look at `gqlgen.yml` and the schema package for other non-generated models.

type JobTag struct {
	ID      string `json:"id" db:"id"`
	TagType string `json:"tagType" db:"tag_type"`
	TagName string `json:"tagName" db:"tag_name"`
}
