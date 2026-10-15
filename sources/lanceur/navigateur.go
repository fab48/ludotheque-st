package main

import (
	"os"
	"path/filepath"
)

// argumentsNavigateur : profil dédié rangé dans le pack, plein écran, son sans clic.
func argumentsNavigateur(fam, url string) []string {
	profil := filepath.Join(base, "runtime", "navigateur-"+fam)
	_ = os.MkdirAll(profil, 0o755)
	if fam == "firefox" {
		// autoriser le son au démarrage, pas d'écran d'accueil
		prefs := `user_pref("media.autoplay.default", 0);
user_pref("media.autoplay.blocking_policy", 0);
user_pref("browser.shell.checkDefaultBrowser", false);
user_pref("browser.aboutwelcome.enabled", false);
user_pref("datareporting.policy.dataSubmissionEnabled", false);
user_pref("browser.sessionstore.resume_from_crash", false);
user_pref("dom.gamepad.enabled", true);
`
		_ = os.WriteFile(filepath.Join(profil, "user.js"), []byte(prefs), 0o644)
		a := []string{"--profile", profil, "--no-remote", "--new-instance"}
		if cfg.Kiosque {
			a = append(a, "--kiosk")
		}
		return append(a, url)
	}
	a := []string{
		"--user-data-dir=" + profil,
		"--no-first-run", "--no-default-browser-check",
		"--autoplay-policy=no-user-gesture-required",
		"--disable-features=Translate,TranslateUI,MediaRouter",
		"--disable-session-crashed-bubble", "--hide-crash-restore-bubble", "--noerrdialogs",
		"--disable-pinch", "--overscroll-history-navigation=0",
		"--check-for-update-interval=31536000",
		// pas d'extensions : Windows en installe d'office dans le profil (Adobe…),
		// avec des chemins de 175 caractères qui empêchent de copier le pack
		"--disable-extensions",
	}
	if cfg.Kiosque {
		a = append(a, "--kiosk")
		if fam == "edge" {
			a = append(a, "--edge-kiosk-type=fullscreen", "--no-first-run")
		}
		return append(a, url)
	}
	return append(a, "--app="+url, "--window-size=1280,760")
}
