# Overview

TL;DR I need a search engine for dub packages for a small thing I'm working on, and wanted a side-by-side comparison
of Postgres (the database I'm using) and Meilisearch (the dedicated search engine I've been eyeing up).

I figured this might be interesting for anyone who knows how to work on Dub, because we can all agree that its
search function basically doesn't even exist.

They are both fed the latest (as of this time of writing) name, description, and readme for each package.

The data is an extracted subset of the package dump data from the main dub website.

# Running

```bash
docker-compose up
```

(note it can take a while for Meilisearch to build its index).

`localhost:8080` is a simple webserver showing the postgres and Meilisearch queries side by side.

`localhost:7700` is meilisearch's own front end, if you just want to explore that.