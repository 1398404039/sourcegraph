package main

import (
	"io/ioutil"

	"github.com/neelance/graphql-go"
	"sourcegraph.com/sourcegraph/sourcegraph/api"
)

func main() {
	json, err := graphql.MustParseSchema(api.Schema, nil).ToJSON()
	if err != nil {
		panic(err)
	}

	if err := ioutil.WriteFile("schema.json", json, 0666); err != nil {
		panic(err)
	}
}
