package environment

import "os"

func GetOpenAIKey() string {
	return os.Getenv("OPENAI_API_KEY") // Simpan API Key di environment variable
}

func GetWebClientId() string {
	return os.Getenv("WEB_APP_CLIENT_ID") // Simpan Client ID di environment variable
}

func GetFirebaseKey() string {
	return os.Getenv("FIREBASE_CREDENTIALS")
}

func GetFirebaseProjectID() string {
	return os.Getenv("FIREBASE_PROJECT_ID")
}
