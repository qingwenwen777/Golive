package service

const defaultAvatarSkinColor = "ffdbb4"

func DefaultAvatarURL(seed string) string {
	return "https://api.dicebear.com/7.x/avataaars/svg?seed=" + urlSafeSeed(seed) + "&skinColor=" + defaultAvatarSkinColor
}
