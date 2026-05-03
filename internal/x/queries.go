package x

import (
	"net/url"
	"strconv"
)

const (
	searchNextToken    = "next_token"
	userPostsNextToken = "pagination_token"
)

func searchQuery(options SearchOptions, limit int) url.Values {
	query := commonPostQuery()
	query.Set("query", options.Query)
	query.Set("max_results", strconv.Itoa(limit))
	if options.NextToken != "" {
		query.Set(searchNextToken, options.NextToken)
	}

	return query
}

func lookupQuery() url.Values {
	return commonPostQuery()
}

func userPostsQuery(options UserPostsOptions, limit int) url.Values {
	query := commonPostQuery()
	query.Set("max_results", strconv.Itoa(limit))
	if options.NextToken != "" {
		query.Set(userPostsNextToken, options.NextToken)
	}

	return query
}

func usernameLookupQuery() url.Values {
	query := url.Values{}
	query.Set("user.fields", "id,username,name")

	return query
}

func commonPostQuery() url.Values {
	query := url.Values{}
	query.Set("expansions", "author_id")
	query.Set("tweet.fields", "created_at,author_id")
	query.Set("user.fields", "username,name")

	return query
}
