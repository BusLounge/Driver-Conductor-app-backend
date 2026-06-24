package handlers

// Image optimization helper functions

// optimizeImageURLs transforms Cloudinary URLs to the requested quality
// Input examples: https://res.cloudinary.com/image.jpg
// Output: https://res.cloudinary.com/c_limit,h_480,q_70,f_auto/image.jpg (for sd)
// f_auto = automatic WebP/JPEG format based on browser support
func optimizeImageURLs(urls []string, quality string) []string {
	if len(urls) == 0 {
		return urls
	}

	optimized := make([]string, 0, len(urls))
	for _, url := range urls {
		optimized = append(optimized, optimizeImageURL(url, quality))
	}
	return optimized
}

// optimizeImageURL transforms a single Cloudinary URL to the requested quality
// Includes format auto-negotiation and progressive encoding
func optimizeImageURL(url string, quality string) string {
	if url == "" {
		return url
	}

	// Detect if it's a Cloudinary URL
	if !contains(url, "cloudinary") {
		return url
	}

	// Skip if already has transformation params
	if contains(url, "/c_") || contains(url, "/q_") {
		return url
	}

	var transformation string
	switch quality {
	case "sd":
		// Standard Definition: 480px max, 70% quality, auto format, progressive
		// f_auto = WebP for modern browsers, JPEG fallback
		// fl_progressive = progressive encoding for faster perceived load
		transformation = "/c_limit,h_480,w_480,q_70,f_auto,fl_progressive/"
	case "hd":
		// High Definition: 720px max, 80% quality, auto format with progressive
		transformation = "/c_limit,h_720,w_720,q_80,f_auto,fl_progressive/"
	case "full":
		// Full quality: auto format only (no dimension limit)
		transformation = "/f_auto,q_90,fl_progressive/"
	default:
		// Default to SD
		transformation = "/c_limit,h_480,w_480,q_70,f_auto,fl_progressive/"
	}

	// Insert transformation after /upload/
	uploadIdx := indexOf(url, "/upload/")
	if uploadIdx == -1 {
		return url
	}

	insertPos := uploadIdx + len("/upload/")
	return url[:insertPos] + transformation + url[insertPos:]
}

// Helper functions
func contains(s string, substr string) bool {
	return indexOf(s, substr) != -1
}

func indexOf(s string, substr string) int {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

