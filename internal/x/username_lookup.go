package x

import (
	"context"
	"net/url"

	"github.com/alnah/moth/internal/content"
)

const usernameLookupOperation = "username lookup"

// UsernameLookupOptions contains X username lookup parameters.
type UsernameLookupOptions struct {
	Username string
}

// LookupUserByUsername returns one normalized X user profile.
func (client *Client) LookupUserByUsername(ctx context.Context, options UsernameLookupOptions) (content.Pack, error) {
	var response xUserLookupResponse
	metadata, err := client.get(
		ctx,
		usernameLookupOperation,
		usernameLookupPath(options.Username),
		usernameLookupQuery(),
		&response,
	)
	if err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    []content.Item{mapUserProfile(response.Data)},
		Metadata: metadata,
	}, nil
}

func usernameLookupPath(username string) string {
	return "/2/users/by/username/" + url.PathEscape(username)
}
