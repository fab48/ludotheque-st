// Ludothèque ST : lanceur portable.
//
// Un seul exécutable, posé à la racine du pack. Il sert l'interface (ui/),
// ouvre le navigateur en plein écran, lance les jeux (Hatari, versions
// natives, versions web) et surveille la touche Échap et la manette pour
// revenir au bac.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const version = "1.3"

// Config : réglages du pack (config.json à la racine).
type Config struct {
	Port             int      `json:"port"`
	Navigateur       string   `json:"navigateur"`       // "auto" ou chemin d'un exécutable
	Kiosque          bool     `json:"kiosque"`          // plein écran sans barre
	PleinEcranJeux   bool     `json:"plein_ecran_jeux"` // Hatari en plein écran
	SortieEchap      float64  `json:"sortie_echap_s"`   // Échap maintenu N s = quitter le jeu
	SortieManette    float64  `json:"sortie_manette_s"` // Select+Start maintenus N s
	BoutonsManette   [][2]int `json:"boutons_manette"`  // paires (Select, Start) pour les manettes non XInput
	HatariWindows    string   `json:"hatari_windows"`   // chemin relatif au pack
	HatariLinux      string   `json:"hatari_linux"`     // chemin relatif au pack, sinon « hatari » du système
	HatariOptions    []string `json:"hatari_options"`   // ajoutées à chaque lancement
	AutoriserSysteme bool     `json:"autoriser_extinction"`
	ChargementRapide bool     `json:"chargement_rapide"`   // lecteur de disquette accéléré, démarrage rapide du TOS
	Joystick         string   `json:"joystick"`            // auto | manette | clavier
	TirClavier2      string   `json:"tir_clavier_j2"`      // tir du joueur 2 au clavier (Z Q S D)
	TirClavier       string   `json:"tir_clavier"`         // touche de tir quand le joystick est au clavier
	ToucheCapture    string   `json:"touche_capture"`      // capture d'écran dans Hatari
	ToucheAuto       bool     `json:"touche_menu_auto"`    // presser seul la touche des menus de compil
	DemosApres       float64  `json:"demos_apres_min"`     // inactivité avant les démos (0 = jamais)
	DureeDemo        float64  `json:"duree_demo_min"`      // durée de chaque démo
	NettoyerAmorce   bool     `json:"nettoyer_amorce"`     // masquer l'écran « Little Green Desktop »
	CarteChargement  bool     `json:"carte_au_chargement"` // commandes en plein écran pendant le turbo
	Turbo            string   `json:"turbo"`               // disque (accéléré pendant les chargements) | debut | non
	TurboChargement  float64  `json:"turbo_chargement_s"`  // mode « debut » : secondes en accéléré au démarrage
	EcranFiltre      string   `json:"ecran_filtre"`        // net | lisse | entier | hatari
	EcranLignes      int      `json:"ecran_lignes"`        // lignes de balayage, 0..100
	Ecran43          bool     `json:"ecran_4_3"`           // proportions d'un écran d'époque
	EcranHz          int      `json:"ecran_hz"`            // écran passé à 50 Hz pendant Hatari (0 = non), voir frequence.go
}

func configParDefaut() Config {
	return Config{
		Port: 8480, Navigateur: "auto", Kiosque: true, PleinEcranJeux: true,
		SortieEchap: 2, SortieManette: 1,
		BoutonsManette:   [][2]int{{6, 7}, {8, 9}},
		HatariWindows:    "emulateurs/hatari-windows/hatari.exe",
		HatariLinux:      "emulateurs/hatari-linux/hatari",
		HatariOptions:    []string{},
		AutoriserSysteme: true,
		ChargementRapide: true, Joystick: "auto", TirClavier: "Right Ctrl", ToucheCapture: "F8",
		ToucheAuto: true, DemosApres: 3, DureeDemo: 4, NettoyerAmorce: true, TurboChargement: 4, CarteChargement: true,
		EcranFiltre: "net", Turbo: "disque", EcranHz: 50,
	}
}

var (
	base    string // dossier du pack
	cfg     Config
	jeux    = &Gestionnaire{}
	diff    = &Diffusion{clients: map[chan string]bool{}}
	fenetre = flag.Bool("fenetre", false, "navigateur en fenêtre (pas de mode kiosque)")
	sansNav = flag.Bool("sans-navigateur", false, "ne lance que le serveur")
	portArg = flag.Int("port", 0, "port local (sinon config.json)")
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "--trier" { // outil : liste des jeux TOSEC à ajouter (tri.go)
		base = dossierPack()
		if err := trierCollection(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--exporter" { // zip prêt à l'emploi : favoris ou bac (export.go)
		base = dossierPack()
		if err := exporterLigneDeCommande(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	flag.Parse()
	base = dossierPack()
	_ = os.MkdirAll(filepath.Join(base, "runtime"), 0o755)
	if f, err := os.OpenFile(filepath.Join(base, "runtime", "ludotheque.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
	log.Printf("Ludothèque ST %s — %s/%s — pack : %s", version, runtime.GOOS, runtime.GOARCH, base)
	cfg = chargerConfig()
	_ = os.Remove(filepath.Join(base, "runtime", "quitter"))
	if *portArg > 0 {
		cfg.Port = *portArg
	}
	if *fenetre {
		cfg.Kiosque = false
	}

	adresse := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	url := "http://" + adresse + "/"
	ln, err := net.Listen("tcp", adresse)
	if err != nil {
		// Déjà lancée : on se contente de rouvrir le navigateur.
		log.Printf("port %d occupé, la ludothèque tourne déjà ? (%v)", cfg.Port, err)
		if !*sansNav {
			ouvrirParDefaut(url)
		}
		return
	}

	http.HandleFunc("/api/catalogue", apiCatalogue)
	http.HandleFunc("/api/vrac", apiVrac)
	http.HandleFunc("/api/lancer", apiLancer)
	http.HandleFunc("/api/arreter", apiArreter)
	http.HandleFunc("/api/fin-web", apiFinWeb)
	http.HandleFunc("/api/evenements", apiEvenements)
	http.HandleFunc("/api/admin", apiAdmin)
	http.HandleFunc("/api/systeme", apiSysteme)
	http.HandleFunc("/api/affichage", apiAffichage)
	http.HandleFunc("/api/demos", apiDemos)
	http.HandleFunc("/api/capture/retirer", apiRetirerCapture)
	http.HandleFunc("/api/demo/demarrer", apiDemoDemarrer)
	http.HandleFunc("/api/demo/arreter", apiDemoArreter)
	http.HandleFunc("/api/demo/suivante", apiDemoSuivante)
	http.HandleFunc("/api/menage", apiMenage)
	http.HandleFunc("/api/exporter", apiExporter)
	http.HandleFunc("/", fichiers)
	srv := &http.Server{Handler: gardien(http.DefaultServeMux)}
	go func() { _ = srv.Serve(ln) }()
	log.Printf("interface : %s", url)

	go surveillerEntrees() // Échap maintenu, Select+Start (selon le système)
	lancerRangement(false) // disques/GAME : catalogue installé et ménage, si son contenu a changé

	if *sansNav {
		select {}
	}
	nav := lancerNavigateur(url)
	if nav == nil {
		log.Printf("aucun navigateur trouvé : ouverture par défaut")
		ouvrirParDefaut(url)
		select {}
	}
	_ = nav.Wait()
	log.Printf("navigateur fermé : arrêt")
	arreterDemos("fermeture")
	jeux.Arreter()
	ctx, annuler := context.WithTimeout(context.Background(), time.Second)
	defer annuler()
	_ = srv.Shutdown(ctx)
}

func dossierPack() string {
	if d := os.Getenv("LUDO_DIR"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		d, _ := os.Getwd()
		return d
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	d := filepath.Dir(exe)
	// Exécutable rangé dans un sous-dossier (ex. sources/) : remonter jusqu'au catalogue.
	for i := 0; i < 3; i++ {
		if _, err := os.Stat(filepath.Join(d, "catalogue.json")); err == nil {
			return d
		}
		d = filepath.Dir(d)
	}
	return filepath.Dir(exe)
}

func chargerConfig() Config {
	c := configParDefaut()
	chemin := filepath.Join(base, "config.json")
	b, err := os.ReadFile(chemin)
	if err != nil {
		out, _ := json.MarshalIndent(c, "", "  ")
		_ = os.WriteFile(chemin, out, 0o644)
		return c
	}
	if err := json.Unmarshal(b, &c); err != nil {
		log.Printf("config.json illisible (%v) : réglages par défaut", err)
		return configParDefaut()
	}
	return c
}

// chemin : résout un chemin relatif au pack, en refusant d'en sortir.
func chemin(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("chemin vide")
	}
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel), nil
	}
	p := filepath.Join(base, filepath.FromSlash(rel))
	r, err := filepath.Rel(base, p)
	if err != nil || strings.HasPrefix(r, "..") {
		return "", errors.New("chemin hors du pack")
	}
	return p, nil
}

func existe(p string) bool { _, err := os.Stat(p); return err == nil }

// gardien : seule l'interface de la ludothèque parle au lanceur. Une page web ouverte
// dans un autre navigateur pouvait poster vers 127.0.0.1 (éteindre le PC, ménage…) ;
// un nom de domaine qui pointerait sur 127.0.0.1 est refusé aussi.
func gardien(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hote := r.Host
		if hh, _, err := net.SplitHostPort(hote); err == nil {
			hote = hh
		}
		if hote != "127.0.0.1" && hote != "localhost" {
			http.Error(w, "hôte refusé", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			log.Printf("requête refusée : %s %s depuis %s", r.Method, r.URL.Path, o)
			http.Error(w, "origine refusée", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// ---------- fichiers statiques ----------

var typesMime = map[string]string{".wav": "audio/wav", ".js": "text/javascript", ".json": "application/json",
	".woff2": "font/woff2", ".ttf": "font/ttf", ".png": "image/png", ".bmp": "image/bmp", ".jpg": "image/jpeg"}

func fichiers(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if p == "/" {
		p = "/ui/index.html"
	}
	ok := false
	for _, pre := range []string{"/ui/", "/medias/", "/reforged/"} {
		if strings.HasPrefix(p, pre) {
			ok = true
		}
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := chemin(strings.TrimPrefix(p, "/"))
	if err != nil || !existe(f) {
		http.NotFound(w, r)
		return
	}
	if t, ok := typesMime[strings.ToLower(filepath.Ext(f))]; ok {
		w.Header().Set("Content-Type", t)
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, f)
}

func repondre(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func erreur(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"erreur": msg})
}

// ---------- évènements (Server-Sent Events) ----------

type Diffusion struct {
	mu      sync.Mutex
	clients map[chan string]bool
}

func (d *Diffusion) Envoyer(typ string, donnees any) {
	b, _ := json.Marshal(map[string]any{"type": typ, "donnees": donnees})
	d.mu.Lock()
	defer d.mu.Unlock()
	for c := range d.clients {
		select {
		case c <- string(b):
		default:
		}
	}
}

func apiEvenements(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "flux impossible", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	c := make(chan string, 8)
	diff.mu.Lock()
	diff.clients[c] = true
	diff.mu.Unlock()
	defer func() { diff.mu.Lock(); delete(diff.clients, c); diff.mu.Unlock() }()
	fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "etat", "donnees": jeux.Etat()}))
	fl.Flush()
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-c:
			fmt.Fprintf(w, "data: %s\n\n", m)
			fl.Flush()
		case <-t.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// ---------- système ----------

func apiSysteme(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&q)
	switch q.Action {
	case "quitter":
		repondre(w, map[string]bool{"ok": true})
		// signal pour la session dédiée Linux : ne pas relancer la ludothèque
		_ = os.WriteFile(filepath.Join(base, "runtime", "quitter"), []byte("1"), 0o644)
		go func() {
			time.Sleep(300 * time.Millisecond)
			jeux.Arreter()
			ecranAvantSortie()
			fermerNavigateur()
			os.Exit(0)
		}()
	case "eteindre", "redemarrer":
		if !cfg.AutoriserSysteme {
			erreur(w, 403, "extinction désactivée dans config.json")
			return
		}
		repondre(w, map[string]bool{"ok": true})
		go func() { time.Sleep(500 * time.Millisecond); eteindre(q.Action == "redemarrer") }()
	default:
		erreur(w, 400, "action inconnue")
	}
}

func eteindre(redemarrer bool) {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		a := "/s"
		if redemarrer {
			a = "/r"
		}
		c = exec.Command("shutdown", a, "/t", "0")
	} else {
		a := "poweroff"
		if redemarrer {
			a = "reboot"
		}
		c = exec.Command("systemctl", a)
	}
	if err := c.Run(); err != nil {
		log.Printf("extinction : %v", err)
	}
}
