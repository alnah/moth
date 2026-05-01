package x

import "github.com/alnah/moth/internal/content"

const xStatusBaseURL = "https://x.com/"

func mapPosts(posts []xPost, users map[string]xUser, limit int) []content.Item {
	items := make([]content.Item, 0, min(len(posts), limit))
	for _, post := range posts {
		if post.ID == "" {
			continue
		}
		items = append(items, mapPost(post, users[post.AuthorID]))
		if len(items) >= limit {
			break
		}
	}

	return items
}

func mapPost(post xPost, author xUser) content.Item {
	username := author.Username
	return content.Item{
		Kind:     content.KindSocialPost,
		URL:      postURL(post.ID, username),
		Title:    postTitle(username, author.Name),
		Text:     post.Text,
		Metadata: postMetadata(post, author),
	}
}

func postURL(postID string, username string) string {
	if username == "" {
		return xStatusBaseURL + "i/web/status/" + postID
	}

	return xStatusBaseURL + username + "/status/" + postID
}

func postTitle(username string, name string) string {
	if username != "" {
		return "@" + username
	}

	return name
}

func postMetadata(post xPost, author xUser) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "post_id", post.ID)
	addStringMetadata(metadata, "author_id", post.AuthorID)
	addStringMetadata(metadata, "author_username", author.Username)
	addStringMetadata(metadata, "author_name", author.Name)
	addStringMetadata(metadata, "created_at", post.CreatedAt)

	return metadata
}

func usersByID(users []xUser) map[string]xUser {
	usersByID := make(map[string]xUser, len(users))
	for _, user := range users {
		if user.ID != "" {
			usersByID[user.ID] = user
		}
	}

	return usersByID
}
