package database

import "go.mongodb.org/mongo-driver/v2/mongo"

type Collections struct {
	Pages *mongo.Collection
}

func NewCollections(db *mongo.Database) *Collections {
	return &Collections{
		Pages: db.Collection("pages"),
	}
}
