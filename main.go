package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/meilisearch/meilisearch-go"
)

func main() {
	if os.Args[1] == "extract-mirror" {
		extractMirror()
	} else if os.Args[1] == "seed" {
		seed()
	} else if os.Args[1] == "serve" {
		serve()
	}
}

type MirrorInfo struct {
	Description string `json:"description"`
}

type MirrorVersion struct {
	Readme string     `json:"readme"`
	Info   MirrorInfo `json:"info"`
}

type MirrorPackage struct {
	Name     string          `json:"name"`
	Versions []MirrorVersion `json:"versions"`
}

type MirrorExtract struct {
	Name        string `json:"name"`
	Readme      string `json:"readme"`
	Description string `json:"description"`
}

func extractMirror() {
	var packages []MirrorPackage
	b, _ := os.ReadFile("./mirror.json")
	json.Unmarshal(b, &packages)

	var extract []MirrorExtract
	for _, v := range packages {
		if len(v.Versions) < 1 {
			continue
		}
		extract = append(extract, MirrorExtract{
			Name:        v.Name,
			Readme:      v.Versions[len(v.Versions)-1].Readme,
			Description: v.Versions[len(v.Versions)-1].Info.Description,
		})
	}

	data, _ := json.Marshal(extract)
	os.WriteFile("mirror-extract.json", data, 0777)
}

func serve() {
	r := mux.NewRouter()
	r.HandleFunc("/search", serveSearch)
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("."))))
	s := &http.Server{
		Handler: r,
		Addr:    "0.0.0.0:8080",
	}
	s.ListenAndServe()
}

func serveSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	a, b := search(query)

	bytes, _ := json.Marshal(struct {
		Postgres    []string `json:"postgres"`
		Meilisearch []string `json:"meilisearch"`
	}{
		Postgres:    a,
		Meilisearch: b,
	})
	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func search(query string) (postgres []string, meili []string) {
	client := meilisearch.NewClient(meilisearch.ClientConfig{
		Host: "http://meili:7700",
	})
	db, err := sql.Open("postgres", "host=postgres port=5432 user=postgres password=test dbname=search sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	rows, _ := db.Query("SELECT name FROM search_packages($1) ORDER BY rank DESC;", query)
	for rows.Next() {
		var str string
		rows.Scan(&str)
		postgres = append(postgres, str)
	}

	res, _ := client.Index("packages").Search(query, &meilisearch.SearchRequest{})
	for _, hit := range res.Hits {
		r, _ := json.Marshal(hit)
		var data MirrorExtract
		json.Unmarshal(r, &data)
		meili = append(meili, data.Name)
	}

	return
}

func seed() {
	client := meilisearch.NewClient(meilisearch.ClientConfig{
		Host: "http://meili:7700",
	})

	db, err := sql.Open("postgres", "host=postgres port=5432 user=postgres password=test dbname=search sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	db.Exec(`
	DROP TABLE IF EXISTS package;
	CREATE TABLE package(
		id              SERIAL PRIMARY KEY,
		name            VARCHAR(256) NOT NULL,
		query_vector    TSVECTOR,
		next_update     TIMESTAMP WITH TIME ZONE
	);
	CREATE INDEX ON package(name);
	CREATE INDEX ON package USING gin(query_vector);
	
	CREATE OR REPLACE FUNCTION update_package_query_vector(in pid int, in description text, in readme text) RETURNS void
	AS $$
		UPDATE package SET query_vector = (to_tsvector(name) || to_tsvector(readme) || to_tsvector(description)) WHERE id = pid;
	$$
	LANGUAGE SQL;
	
	CREATE OR REPLACE FUNCTION search_packages(in query text) RETURNS TABLE(id int, name text, rank real)
	AS $$
		SELECT DISTINCT ON (id)
			id, name, SUM(rank) AS rank
		FROM
		(
			SELECT 
				id, name, 10 AS rank 
			FROM 
				package 
			WHERE 
				name = query
			UNION ALL
			(
				SELECT
					id, name, 1 AS rank
				FROM
					package
				WHERE
					name LIKE (query || '%')
					OR
					name LIKE ('%' || query)
			)
			UNION ALL
			(
				SELECT 
					id, name, ts_rank_cd(query_vector, to_tsquery(query)) AS rank 
				FROM 
					package 
				WHERE 
					query_vector @@ to_tsquery(query)
			)
		) AS _
		GROUP BY id, name;
	$$
	LANGUAGE SQL;
	`)

	packages := client.Index("packages")
	b, err := os.ReadFile("/data.json")
	if err != nil {
		log.Fatal(err)
	}

	var data []MirrorExtract
	json.Unmarshal(b, &data)

	for _, v := range data {
		_, err = packages.AddDocuments([1]MirrorExtract{v})
		if err != nil {
			log.Fatal(err)
		}
		db.Exec("INSERT INTO package(name) VALUES($1);", v.Name)
		var id int
		db.QueryRow("SELECT id FROM package WHERE name = $1", v.Name).Scan(&id)
		_, err = db.Exec("SELECT * FROM update_package_query_vector($1, $2, $3);", id, v.Description, v.Readme)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Print("Done")
}
