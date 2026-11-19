package storage

import "fmt"

// Object key layout (architecture §6).
func OriginalKey(spaceID, albumID, mediaID string) string {
	return fmt.Sprintf("spaces/%s/albums/%s/originals/%s", spaceID, albumID, mediaID)
}

func ThumbKey(spaceID, albumID, mediaID string) string {
	return fmt.Sprintf("spaces/%s/albums/%s/thumbs/%s.jpg", spaceID, albumID, mediaID)
}

func PreviewKey(spaceID, albumID, mediaID string) string {
	return fmt.Sprintf("spaces/%s/albums/%s/previews/%s.jpg", spaceID, albumID, mediaID)
}
