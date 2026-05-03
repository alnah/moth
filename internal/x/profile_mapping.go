package x

import "github.com/alnah/moth/internal/content"

func mapUserProfile(user xUser) content.Item {
	return content.Item{
		Kind:     content.KindSocialProfile,
		URL:      userProfileURL(user.Username),
		Title:    userProfileTitle(user),
		Metadata: userProfileMetadata(user),
	}
}

func userProfileURL(username string) string {
	if username == "" {
		return ""
	}

	return xWebBaseURL + username
}

func userProfileTitle(user xUser) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	if user.Name != "" {
		return user.Name
	}

	return user.ID
}

func userProfileMetadata(user xUser) map[string]any {
	metadata := map[string]any{"source": "x"}
	addStringMetadata(metadata, "user_id", user.ID)
	addStringMetadata(metadata, "username", user.Username)
	addStringMetadata(metadata, "name", user.Name)

	return metadata
}
