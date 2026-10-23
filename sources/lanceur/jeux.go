package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- catalogue ----------

type Variante struct {
	Type       string      `json:"type"` // hatari | natif | web
	Nom        string      `json:"nom"`
	Desc       string      `json:"desc,omitempty"`
	Disques    []string    `json:"disques,omitempty"`
	Machine    string      `json:"machine,omitempty"`
	Tos        string      `json:"tos,omitempty"`
	Auto       string      `json:"auto,omitempty"`       // programme lancé seul (Hatari 2.x)
	Correctifs []Correctif `json:"correctifs,omitempty"` // octets remplacés dans une copie de l'image
	Dossier    string      `json:"dossier,omitempty"`    // dossier monté en disque dur C: (GEMDOS, Hatari 2.x)
	Options    []string    `json:"options,omitempty"`
	Windows    []string    `json:"windows,omitempty"` // commande native Windows
	Linux      []string    `json:"linux,omitempty"`   // commande native Linux
	Disque     string      `json:"disque,omitempty"`  // remplace {disque} dans la commande
	URL        string      `json:"url,omitempty"`
	Dispo      bool        `json:"dispo"`            // calculé
	Absent     string      `json:"absent,omitempty"` // calculé : ce qui manque
}

type Jeu struct {
	ID          string            `json:"id"`
	Titre       string            `json:"titre"`
	Editeur     string            `json:"editeur,omitempty"`
	Annee       int               `json:"annee,omitempty"`
	Genre       string            `json:"genre,omitempty"`
	Joueurs     int               `json:"joueurs,omitempty"`
	Auteur      string            `json:"auteur,omitempty"`
	Touche      string            `json:"touche,omitempty"` // menu de compil : touche à presser
	ToucheDelai float64           `json:"touche_delai,omitempty"`
	ToucheDuree float64           `json:"touche_duree,omitempty"` // secondes d'appuis répétés (5 par défaut)
	Sequence    []string          `json:"sequence,omitempty"`     // touches pressées à tour de rôle au démarrage (voir codeTouche)
	Commandes   string            `json:"commandes,omitempty"`    // joystick (défaut) | souris
	Aide        string            `json:"aide,omitempty"`         // rappel des commandes du jeu
	ToucheType  string            `json:"touche_type,omitempty"`  // pave (défaut) ou chiffre (rangée du haut)
	Note        string            `json:"note,omitempty"`
	Mhz16       string            `json:"mhz16,omitempty"`      // avis sur le 16 MHz : conseille | risque
	Mhz16Note   string            `json:"mhz16_note,omitempty"` // précision affichée sur la fiche
	Actif       *bool             `json:"actif,omitempty"`
	Favori      *bool             `json:"favori,omitempty"`       // ★ : montré par le filtre « favoris » du bac
	SansTouches bool              `json:"sans_touches,omitempty"` // touches de démarrage coupées (case de la fiche)
	Bords       bool              `json:"bords,omitempty"`        // garde les bords de l'écran ST (jeu en overscan)
	Disquette   map[string]string `json:"disquette,omitempty"`
	Variantes   []Variante        `json:"variantes"`
	Captures    []string          `json:"captures,omitempty"` // calculé
	Vrac        bool              `json:"vrac,omitempty"`     // calculé
	Adulte      bool              `json:"adulte,omitempty"`   // caché du bac sauf Konami code (vrac : calculé, estAdulte)
}

type Catalogue struct {
	Titre  string `json:"titre"`
	Hatari struct {
		Machine string   `json:"machine"`
		Tos     string   `json:"tos"`
		Options []string `json:"options"`
	} `json:"hatari"`
	Vrac bool `json:"vrac"` // afficher les disquettes hors catalogue
	// jeux du vrac marqués ★ (bouton de la fiche) : 1re disquette → toutes ses disquettes,
	// de quoi les montrer au démarrage sans attendre le tri de tout le vrac
	FavorisVrac map[string][]string `json:"favoris_vrac,omitempty"`
	// jeux du vrac dont la case « Touches au démarrage » est décochée (1re disquette) :
	// l'Espace qui passe l'intro du crack choisissait les menus de certains (Sherman M4)
	SansTouchesVrac []string `json:"sans_touches_vrac,omitempty"`
	Jeux            []Jeu    `json:"jeux"`
}

var muCatalogue sync.Mutex

func lireCatalogue() (*Catalogue, error) {
	b, err := os.ReadFile(filepath.Join(base, "catalogue.json"))
	if err != nil {
		return nil, err
	}
	c := &Catalogue{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("catalogue.json : %w", err)
	}
	return c, nil
}

func ecrireCatalogue(c *Catalogue) error {
	for i := range c.Jeux { // ne pas enregistrer les champs calculés
		c.Jeux[i].Captures = nil
		for k := range c.Jeux[i].Variantes {
			c.Jeux[i].Variantes[k].Dispo = false
			c.Jeux[i].Variantes[k].Absent = ""
		}
	}
	b, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(base, "catalogue.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	_ = os.Rename(filepath.Join(base, "catalogue.json"), filepath.Join(base, "runtime", "catalogue.precedent.json"))
	return os.Rename(tmp, filepath.Join(base, "catalogue.json"))
}

var extImages = map[string]bool{".st": true, ".msa": true, ".stx": true, ".dim": true, ".ipf": true, ".ctr": true, ".zip": true}
var reDisque = regexp.MustCompile(`(?i)\s*\(disk\s*(\d+)\s*of\s*(\d+)\)`)
var reEntre = regexp.MustCompile(`\s*[\(\[].*$`)
var reCompil = regexp.MustCompile(`(?i)^a_(\d+)(?:(?:_[0-9a-z])*_(.+))?$`)

// disquette d'une compil Automation : « A_483_B_monkeyisland », « A_069_5_kickoff2 »
var reCompilDisque = regexp.MustCompile(`(?i)^(a_\d+)_([0-9a-z])_(.+)$`)

// ecartesVrac : parmi les images du vrac au nom TOSEC, celles que le ménage effacerait :
// autres éditions d'un même jeu (une seule reste, choisie comme au ménage), jeux déjà au
// catalogue, titres écartés (démo jouable, données, [b] seulement…). Rien n'est effacé :
// le bac ne les montre pas, c'est tout. Utile pour les images posées hors de disques/GAME
// (racine de disques/…), que le ménage ne touche jamais.
// memeJeu : disquettes de l'édition gardée → clé commune (« (Disk 1 of 2)[!] » et
// « (Disk 2 of 2) », « (King1) » et « (King2) » : un seul jeu).
func ecartesVrac(rels []string) (ecartees map[string]bool, memeJeu map[string]string) {
	var images []*imageTosec
	for _, rel := range rels {
		if e := lireNomTosec(path.Base(rel)); e != nil {
			e.Chemin, e.Source = rel, sourceTosec(path.Dir(rel), e.Nom)
			images = append(images, e)
		}
	}
	absolu := func(i *imageTosec) string { return filepath.Join(base, filepath.FromSlash(i.Chemin)) }
	ecartees, memeJeu = map[string]bool{}, map[string]string{}
	for _, t := range trierTosec(images, absolu, clesCatalogue().complets) {
		garder := map[*imageTosec]bool{}
		if t.Choisie != nil && !t.Catalogue {
			im := t.Choisie.Images
			premier := strings.TrimSuffix(im[0].Nom, filepath.Ext(im[0].Nom))
			cle := filepath.Join(path.Dir(im[0].Chemin), reSansDisque.ReplaceAllString(premier, ""))
			for _, i := range im {
				garder[i] = true
				if len(im) > 1 {
					memeJeu[i.Chemin] = cle
				}
			}
		}
		for _, i := range t.Toutes {
			if !garder[i] {
				ecartees[i.Chemin] = true
			}
		}
	}
	return ecartees, memeJeu
}

// jeux pour adultes de la collection TOSEC : gardés, mais l'interface ne les montre
// qu'après le Konami code (↑ ↑ ↓ ↓ ← → ← → B A)
var reAdulte = regexp.MustCompile(`(?i)\bstrip\b|\bsex|fuck|\blust\b|\bporn|\berot|playboy|centerfold|emmanuelle|teenage queen|blue angel 69|bride of the robot|^fascination$|^geisha$|hollywood poker|piss flaps|\btwat\b`)
var reEditeurAdulte = regexp.MustCompile(`(?i)free spirit|sexy technologies`)

func estAdulte(titre, editeur string) bool {
	titre = strings.ReplaceAll(titre, "_", " ") // nom de fichier hors TOSEC : « Strip_Poker.st »
	return reAdulte.MatchString(titre) || reEditeurAdulte.MatchString(editeur)
}

// vrac : les images de disques/ que le catalogue ne cite pas, regroupées par jeu.
func vrac(c *Catalogue) []Jeu {
	cites := map[string]bool{}
	for _, j := range c.Jeux {
		for _, v := range j.Variantes {
			for _, d := range v.Disques {
				cites[strings.ToLower(filepath.ToSlash(d))] = true
			}
			if v.Disque != "" { // image originale d'une version native (Stunt Car Racer Reforged)
				cites[strings.ToLower(filepath.ToSlash(v.Disque))] = true
			}
		}
	}
	var rels []string
	racine := filepath.Join(base, "disques")
	_ = filepath.WalkDir(racine, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !extImages[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		rel = filepath.ToSlash(rel)
		if !cites[strings.ToLower(rel)] {
			rels = append(rels, rel)
		}
		return nil
	})
	ecartees, memeJeu := ecartesVrac(rels)
	groupes := map[string][]string{}
	for _, rel := range rels {
		if ecartees[rel] {
			continue
		}
		cle := cleVrac(rel)
		if k, ok := memeJeu[rel]; ok {
			cle = k
		}
		groupes[cle] = append(groupes[cle], rel)
	}
	// « vrac »: [] et non null pour l'interface, même sans disquette
	out := []Jeu{}
	numero := func(d string) int { // « Disk 10 of 12 » après « Disk 2 of 12 »
		nom := strings.TrimSuffix(filepath.Base(d), filepath.Ext(d))
		if e := lireNomTosec(filepath.Base(d)); e != nil {
			return e.Disque
		}
		if m := reCompilDisque.FindStringSubmatch(nom); m != nil { // A_483_B : 2 ; A_069_5 : 5
			if n, err := strconv.Atoi(m[2]); err == nil {
				return n
			}
			return int(strings.ToUpper(m[2])[0]-'A') + 1
		}
		return 0
	}
	for cle, ds := range groupes {
		sort.Strings(ds)
		sort.SliceStable(ds, func(a, b int) bool { return numero(ds[a]) < numero(ds[b]) })
		j := jeuVrac(cle, ds)
		if _, ok := c.FavorisVrac[ds[0]]; ok {
			j.Favori = oui()
		}
		j.SansTouches = slices.Contains(c.SansTouchesVrac, ds[0])
		out = append(out, j)
	}
	sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a].Titre) < strings.ToLower(out[b].Titre) })
	return out
}

func oui() *bool { v := true; return &v }

// cleVrac : clé de regroupement des disquettes d'un même jeu du vrac (rel : chemin dans le pack).
func cleVrac(rel string) string {
	nom := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	if lireNomTosec(nom) != nil { // « (Disk 1 of 3)(Intro) », « (Disk B) »
		return filepath.Join(filepath.Dir(rel), reSansDisque.ReplaceAllString(nom, ""))
	}
	if m := reCompilDisque.FindStringSubmatch(nom); m != nil {
		return filepath.Join(filepath.Dir(rel), m[1]+"_"+m[3])
	}
	return filepath.Join(filepath.Dir(rel), reDisque.ReplaceAllString(nom, ""))
}

// jeuVrac : la fiche d'un jeu du vrac, d'après ses disquettes (dans l'ordre) et leur clé.
func jeuVrac(cle string, ds []string) Jeu {
	nom := filepath.Base(cle)
	titre := strings.TrimSpace(reEntre.ReplaceAllString(nom, ""))
	annee := 0
	if m := regexp.MustCompile(`\((19\d\d)\)`).FindStringSubmatch(nom); m != nil {
		fmt.Sscan(m[1], &annee)
	}
	genre := "Vrac"
	if m := reCompil.FindStringSubmatch(nom); m != nil {
		t := strings.ReplaceAll(m[2], "_", " ")
		if t == "" {
			t = "compil " + m[1]
		}
		titre = strings.Title(t) + " (Automation " + m[1] + ")"
		genre = "Compil"
	}
	editeur, machine := "", ""
	var sequence []string
	if e := lireNomTosec(filepath.Base(ds[0])); e != nil { // nom TOSEC : « Bard's Tale, The (1987)(Electronic Arts)… »
		titre, editeur = titreLisible(e.Titre), e.Editeur
		annee, _ = strconv.Atoi(e.Annee)
		if e.Machine == "ste" {
			machine = "ste"
		}
		// comme les jeux TOSEC du catalogue : Espace passe l'écran du crack ;
		// « Trainer Y/N ? » : N
		sequence = []string{"espace"}
		if drapeau(e, "t") {
			sequence = []string{"N", "espace"}
		}
	}
	if titre == "" {
		titre = nom
	}
	desc := "Hatari"
	if len(ds) > 1 {
		desc = fmt.Sprintf("Hatari, %d disquettes", len(ds))
	}
	return Jeu{ID: "vrac:" + ds[0], Titre: titre, Editeur: editeur, Annee: annee, Genre: genre, Vrac: true, Adulte: estAdulte(titre, editeur), Sequence: sequence,
		Variantes: []Variante{{Type: "hatari", Nom: "Original ST", Desc: desc, Disques: ds, Machine: machine}}}
}

// favorisVrac : les jeux du vrac marqués ★, sans trier tout le vrac (premier affichage du
// bac : le tri des 2 000 disquettes du vrac arrive ensuite, par /api/vrac).
func favorisVrac(c *Catalogue) []Jeu {
	out := []Jeu{}
	for id, ds := range c.FavorisVrac {
		if len(ds) == 0 || ds[0] != id {
			continue
		}
		if p, err := chemin(id); err != nil || !existe(p) {
			continue
		}
		j := jeuVrac(cleVrac(id), ds)
		j.Favori = oui()
		j.SansTouches = slices.Contains(c.SansTouchesVrac, id)
		out = append(out, j)
	}
	return out
}

func trouverJeu(c *Catalogue, id string) (*Jeu, bool) {
	for i := range c.Jeux {
		if c.Jeux[i].ID == id {
			return &c.Jeux[i], true
		}
	}
	if strings.HasPrefix(id, "vrac:") {
		for _, j := range vrac(c) {
			if j.ID == id {
				jj := j
				return &jj, true
			}
		}
	}
	return nil, false
}

// completer : disponibilité de chaque variante (Hatari, TOS, images, programmes)
// et captures présentes. tosDefaut : le TOS du catalogue quand la variante n'en
// impose pas.
func completer(j *Jeu, hatariOK bool, tosDefaut string) {
	for k := range j.Variantes {
		v := &j.Variantes[k]
		v.Dispo, v.Absent = true, ""
		switch v.Type {
		case "hatari":
			if !hatariOK {
				v.Dispo, v.Absent = false, "Hatari introuvable"
			}
			if t := firstNonEmpty(v.Tos, tosDefaut); t != "" {
				if p, err := chemin(t); err != nil || !existe(p) {
					v.Dispo, v.Absent = false, "TOS absent : "+t
				}
			}
			for _, d := range v.Disques {
				if p, err := chemin(d); err != nil || !existe(p) {
					v.Dispo, v.Absent = false, "image absente : "+filepath.Base(d)
				}
			}
			if v.Dossier != "" {
				if p, err := chemin(v.Dossier); err != nil || !existe(p) {
					v.Dispo, v.Absent = false, "à déposer : "+v.Dossier
				} else if _, v2 := hatariExe(); !v2 {
					v.Dispo, v.Absent = false, "demande Hatari 2.x"
				}
			}
		case "natif":
			cmd := v.Linux
			if runtime.GOOS == "windows" {
				cmd = v.Windows
			}
			if len(cmd) == 0 {
				v.Dispo, v.Absent = false, "pas de version pour ce système"
			} else if p, err := chemin(cmd[0]); err != nil || !existe(p) {
				v.Dispo, v.Absent = false, "à déposer : "+cmd[0]
			} else if v.Disque != "" {
				if p, err := chemin(v.Disque); err != nil || !existe(p) {
					v.Dispo, v.Absent = false, "image originale à déposer : "+v.Disque
				}
			}
		case "web":
			if p, err := chemin(v.URL); err != nil || !existe(p) {
				v.Dispo, v.Absent = false, "à déposer : "+v.URL
			}
		}
	}
	j.Captures = nil
	if j.Vrac {
		return
	}
	// Tes captures (AltGr+G en jeu) passent devant celles fournies avec le pack,
	// la plus récente en premier.
	dir := filepath.Join(base, "medias", j.ID)
	ents, _ := os.ReadDir(dir)
	type im struct {
		nom   string
		perso bool
		date  time.Time
	}
	var ims []im
	for _, e := range ents {
		x := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || !(x == ".png" || x == ".jpg" || x == ".bmp" || x == ".gif" || x == ".webp") {
			continue
		}
		info, _ := e.Info()
		var d time.Time
		if info != nil {
			d = info.ModTime()
		}
		ims = append(ims, im{e.Name(), strings.HasPrefix(strings.ToLower(e.Name()), "grab"), d})
	}
	sort.SliceStable(ims, func(a, b int) bool {
		if ims[a].perso != ims[b].perso {
			return ims[a].perso
		}
		if ims[a].perso {
			return ims[a].date.After(ims[b].date)
		}
		return ims[a].nom < ims[b].nom
	})
	for _, x := range ims {
		j.Captures = append(j.Captures, "medias/"+j.ID+"/"+x.nom)
	}
}

func apiCatalogue(w http.ResponseWriter, r *http.Request) {
	muCatalogue.Lock()
	c, err := lireCatalogue()
	muCatalogue.Unlock()
	if err != nil {
		erreur(w, 500, err.Error())
		return
	}
	exe, v2 := hatariExe()
	hOK := exe != ""
	for i := range c.Jeux {
		completer(&c.Jeux[i], hOK, c.Hatari.Tos)
	}
	// ?vrac=favoris : seulement les favoris du vrac, le bac s'affiche sans attendre le tri
	// de tout le vrac (l'interface le demande ensuite à /api/vrac)
	partiel := r.URL.Query().Get("vrac") == "favoris"
	var vr []Jeu
	if partiel {
		vr = favorisVrac(c)
	} else {
		vr = vrac(c)
	}
	for i := range vr {
		completer(&vr[i], hOK, c.Hatari.Tos)
	}
	repondre(w, map[string]any{
		"titre": c.Titre, "vrac_actif": c.Vrac, "jeux": c.Jeux, "vrac": vr, "vrac_complet": !partiel,
		"systeme": map[string]any{"os": runtime.GOOS, "manettes": nombreManettes(), "version": version, "hatari": exe, "hatari2": v2, "capture": texteCapture(),
			"sortie_echap_s": cfg.SortieEchap, "sortie_manette_s": cfg.SortieManette,
			"extinction": cfg.AutoriserSysteme, "affichage": etatAffichage()},
		"rangement": etatRangement(),
		"etat":      jeux.Etat(),
	})
}

// apiVrac : tout le vrac, trié et complété (après /api/catalogue?vrac=favoris).
func apiVrac(w http.ResponseWriter, r *http.Request) {
	muCatalogue.Lock()
	c, err := lireCatalogue()
	muCatalogue.Unlock()
	if err != nil {
		erreur(w, 500, err.Error())
		return
	}
	exe, _ := hatariExe()
	vr := vrac(c)
	for i := range vr {
		completer(&vr[i], exe != "", c.Hatari.Tos)
	}
	repondre(w, map[string]any{"vrac": vr})
}

// apiAdmin : catalogue (F10) et ★ de la fiche. {"actifs": {"id": true|false},
// "favoris": {"id": true|false}, "vrac": bool} ; un id « vrac:… » est un jeu du vrac.
func apiAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		erreur(w, 405, "POST attendu")
		return
	}
	var q struct {
		Actifs  map[string]bool `json:"actifs"`
		Favoris map[string]bool `json:"favoris"`
		Touches map[string]bool `json:"touches"` // case « Touches au démarrage » de la fiche
		Vrac    *bool           `json:"vrac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		erreur(w, 400, err.Error())
		return
	}
	muCatalogue.Lock()
	defer muCatalogue.Unlock()
	c, err := lireCatalogue()
	if err != nil {
		erreur(w, 500, err.Error())
		return
	}
	for i := range c.Jeux {
		if a, ok := q.Actifs[c.Jeux[i].ID]; ok {
			v := a
			c.Jeux[i].Actif = &v
		}
		if f, ok := q.Favoris[c.Jeux[i].ID]; ok {
			v := f
			c.Jeux[i].Favori = &v
		}
		if t, ok := q.Touches[c.Jeux[i].ID]; ok {
			c.Jeux[i].SansTouches = !t
		}
	}
	for id, t := range q.Touches {
		if rel, duVrac := strings.CutPrefix(id, "vrac:"); duVrac {
			c.SansTouchesVrac = slices.DeleteFunc(c.SansTouchesVrac, func(s string) bool { return s == rel })
			if !t {
				c.SansTouchesVrac = append(c.SansTouchesVrac, rel)
			}
		}
	}
	for id, f := range q.Favoris {
		rel, duVrac := strings.CutPrefix(id, "vrac:")
		switch {
		case !duVrac:
		case !f:
			delete(c.FavorisVrac, rel)
		default: // ses disquettes, pour le montrer au démarrage sans trier le vrac
			if j, ok := trouverJeu(c, id); ok && len(j.Variantes) > 0 {
				if c.FavorisVrac == nil {
					c.FavorisVrac = map[string][]string{}
				}
				c.FavorisVrac[rel] = j.Variantes[0].Disques
			}
		}
	}
	if q.Vrac != nil {
		c.Vrac = *q.Vrac
	}
	if err := ecrireCatalogue(c); err != nil {
		erreur(w, 500, err.Error())
		return
	}
	log.Printf("catalogue (F10) : enregistré")
	repondre(w, map[string]bool{"ok": true})
}

// ---------- Hatari ----------

// hatariExe : l'exécutable Hatari et s'il s'agit d'une version 2.x (--auto, --screenshot-dir).
func hatariExe() (string, bool) {
	if runtime.GOOS == "windows" {
		cands := []string{cfg.HatariWindows}
		m, _ := filepath.Glob(filepath.Join(base, "emulateurs", "hatari-windows", "*", "hatari.exe"))
		for _, x := range m {
			r, _ := filepath.Rel(base, x)
			cands = append(cands, r)
		}
		for _, c := range cands {
			if p, err := chemin(c); err == nil && existe(p) {
				dir := filepath.Dir(p)
				return p, existe(filepath.Join(dir, "SDL2.dll")) || existe(filepath.Join(dir, "SDL3.dll"))
			}
		}
		return "", false
	}
	if p, err := chemin(cfg.HatariLinux); err == nil && existe(p) {
		if ok, v2 := versionHatari(p); ok {
			return p, v2
		}
	}
	if p, err := exec.LookPath("hatari"); err == nil {
		if ok, v2 := versionHatari(p); ok {
			return p, v2
		}
	}
	return "", false
}

type infoVersion struct{ ok, v2 bool }

var (
	muVersion    sync.Mutex // /api/catalogue, /api/lancer et les démos peuvent demander en même temps
	cacheVersion = map[string]infoVersion{}
)

// versionHatari : le binaire démarre-t-il ici, et est-ce une version 2.x ?
func versionHatari(exe string) (bool, bool) {
	muVersion.Lock()
	defer muVersion.Unlock()
	if v, ok := cacheVersion[exe]; ok {
		return v.ok, v.v2
	}
	c := exec.Command(exe, "--version")
	c.Env = os.Environ()
	if d := filepath.Dir(exe); existe(filepath.Join(d, "libSDL2-2.0.so.0")) {
		c.Env = append(c.Env, "LD_LIBRARY_PATH="+d)
	}
	out, err := c.CombinedOutput()
	ok := err == nil || regexp.MustCompile(`Hatari v\d`).Match(out)
	if !ok {
		log.Printf("Hatari %s ne démarre pas ici : %v", exe, err)
	}
	v := infoVersion{ok, !regexp.MustCompile(`v1\.\d`).Match(out)}
	cacheVersion[exe] = v
	return v.ok, v.v2
}

// lancementHatari : tout ce qu'il faut pour démarrer Hatari (jeu ou démo).
type lancementHatari struct {
	machine, tos  string
	disques       []string
	auto, dossier string
	options       []string
	medias        string // dossier des captures
	pleinEcran    bool
	bords         bool   // bords de l'écran ST montrés (démos, jeux en overscan)
	turbo         bool   // démarrage en accéléré
	commandes     string // joystick (défaut) | souris
	correctifs    []Correctif
}

// mhz16 : processeur à 16 MHz (comme un Mega STE) au lieu des 8 MHz d'un ST.
func commandeHatari(c *Catalogue, j *Jeu, v *Variante, mhz16 bool) (*exec.Cmd, string, error) {
	medias := filepath.Join(base, "medias", j.ID)
	if j.Vrac {
		medias = filepath.Join(base, "medias", "_vrac")
	}
	opts := append(append([]string{}, c.Hatari.Options...), v.Options...)
	if mhz16 { // après --machine, qui remet 8 MHz
		opts = append(opts, "--cpuclock", "16")
	}
	return preparerHatari(lancementHatari{
		machine: firstNonEmpty(v.Machine, c.Hatari.Machine, "ste"), tos: firstNonEmpty(v.Tos, c.Hatari.Tos),
		disques: v.Disques, auto: v.Auto, dossier: v.Dossier, options: opts, medias: medias,
		pleinEcran: cfg.PleinEcranJeux, bords: j.Bords, turbo: true, commandes: j.Commandes, correctifs: v.Correctifs,
	})
}

// preparerHatari : construit la commande, écrit la configuration du pack et renvoie
// le chemin du tube de commandes (Linux) pour y envoyer des touches.
func preparerHatari(l lancementHatari) (*exec.Cmd, string, error) {
	exe, v2 := hatariExe()
	if exe == "" {
		return nil, "", errors.New("Hatari introuvable : voir emulateurs/LISEZMOI.txt")
	}
	home := filepath.Join(base, "runtime", "hatari")
	_ = os.MkdirAll(home, 0o755)
	// Sous Windows, Hatari tourne dans le dossier du pack et ne reçoit que des chemins
	// relatifs : SDL lui passe ses arguments en UTF-8 alors qu'il ouvre les fichiers
	// dans l'ancien jeu de caractères de Windows ; un accent dans le dossier du pack
	// l'empêchait de trouver le TOS (console d'erreur cachée derrière le plein écran).
	// Hatari 1.x range ses captures dans le dossier courant : celui des captures.
	dir := l.medias
	if v2 {
		dir = base
	}
	rel := func(p string) string {
		if runtime.GOOS != "windows" {
			return p
		}
		if r, err := filepath.Rel(dir, p); err == nil {
			return r
		}
		return p
	}
	var args []string
	if v2 {
		cfgPath := filepath.Join(home, "ludotheque.cfg")
		_ = os.WriteFile(cfgPath, []byte(configHatari()), 0o644)
		args = append(args, "--configfile", rel(cfgPath))
	}
	// TOS 1.06 / 1.62 n'existent que sur STE : Hatari bloquerait sur une alerte
	if l.machine == "st" || l.machine == "megast" {
		if b := strings.ToLower(filepath.Base(l.tos)); strings.Contains(b, "162") || strings.Contains(b, "106") {
			l.machine = "ste"
		}
	}
	args = append(args, "--machine", l.machine, "--alert-level", "fatal")
	if l.tos != "" {
		p, err := chemin(l.tos)
		if err != nil || !existe(p) {
			return nil, "", fmt.Errorf("TOS absent : %s", l.tos)
		}
		args = append(args, "--tos", rel(p))
	}
	// Disquettes 1 et 2 dans A et B ; toutes vont dans la liste du changement
	// automatique (notre Hatari met la bonne quand le jeu cherche un fichier).
	var images, vrais []string
	prises := map[string]bool{} // copies déjà données à une disquette de ce lancement
	for i, d := range l.disques {
		p, err := chemin(d)
		if err != nil || !existe(p) {
			if i > 1 {
				log.Printf("disquette %d absente, pas de changement automatique vers elle : %s", i+1, d)
				continue
			}
			return nil, "", fmt.Errorf("image absente : %s", d)
		}
		p = disquePropre(p, l.correctifs, prises)
		prises[p] = true
		images, vrais = append(images, rel(p)), append(vrais, p)
		if i < 2 {
			args = append(args, map[int]string{0: "--disk-a", 1: "--disk-b"}[i], rel(p))
		}
	}
	// Une image qui manque fait ouvrir à Hatari une console « Not a disk image » qui
	// attend Entrée, derrière le plein écran : on s'arrête avant.
	for _, p := range vrais {
		if !existe(p) {
			return nil, "", fmt.Errorf("image absente : %s", rel(p))
		}
	}
	liste := ""
	if v2 {
		liste = listeDisquettes(images, vrais)
	}
	if l.pleinEcran {
		args = append(args, "--fullscreen")
	}
	// Port 1 : joueur 1 (manette 1 ou flèches). Port 0 (prise de la souris) :
	// joueur 2 (manette 2 ou Z Q S D) pour les jeux au joystick ; la souris
	// reste branchée pour les jeux qui s'en servent.
	n := nombreManettes()
	j1, j0 := "real", "real"
	if n < 1 {
		j1 = "keys"
	}
	if n < 2 {
		j0 = "keys"
	}
	args = append(args, "--confirm-quit", "no", "--statusbar", "no", "--joy1", j1)
	if l.commandes != "souris" {
		args = append(args, "--joy0", j0)
	}
	if cfg.ChargementRapide {
		args = append(args, "--fastfdc", "yes", "--fast-boot", "yes")
	}
	if l.turbo && v2 && turboMode() == "debut" && cfg.TurboChargement > 0 {
		args = append(args, "--fast-forward", "yes")
	}
	_ = os.MkdirAll(l.medias, 0o755)
	fifo := ""
	if v2 {
		// Pendant l'accéléré, une touche tenue 120 ms dure une seconde pour le ST : le TOS
		// la répète en cliquant, et si le jeu prend le clavier avant le relâché (Espace
		// des touches de démarrage, Rick Dangerous), le clic se répète sans fin. Hatari
		// relâche alors chaque touche au bout d'une image.
		args = append(args, "--fast-forward-key-repeat", "no")
		args = append(args, "--drive-led", "no", "--screenshot-dir", rel(l.medias))
		args = append(args, "--log-file", rel(filepath.Join(home, "hatari.log")))
		args = append(args, argsAffichage(l.pleinEcran, l.bords)...)
		if l.dossier != "" {
			d, err := chemin(l.dossier)
			if err != nil || !existe(d) {
				return nil, "", fmt.Errorf("dossier absent : %s", l.dossier)
			}
			args = append(args, "--harddrive", rel(d), "--gemdos-drive", "C")
		}
		if l.auto != "" {
			args = append(args, "--auto", l.auto)
		}
		if runtime.GOOS != "windows" {
			fifo = filepath.Join(home, "commandes.fifo")
			_ = os.Remove(fifo)
			args = append(args, "--cmd-fifo", fifo)
		}
	} else if l.dossier != "" {
		return nil, "", errors.New("cette version demande Hatari 2.x")
	}
	for _, o := range append(append([]string{}, cfg.HatariOptions...), l.options...) {
		args = append(args, strings.ReplaceAll(o, "{pack}", rel(base)))
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = cheminCourt(dir) // Windows : nom court (8.3) si le dossier a des caractères hors ASCII
	cmd.Env = append(os.Environ(), "HOME="+rel(home), "XDG_CONFIG_HOME="+rel(filepath.Join(home, ".config")))
	cmd.Env = append(cmd.Env, envAffichage()...)
	if l.turbo && v2 && turboMode() == "disque" {
		// notre Hatari accélère seul pendant les chargements (moteur du
		// lecteur, décompactage), les intros restent à vitesse normale
		cmd.Env = append(cmd.Env, "HATARI_LUDO_TURBO=disque")
	}
	if liste != "" {
		cmd.Env = append(cmd.Env, "HATARI_LUDO_DISQUES="+rel(liste))
		log.Printf("changement de disquette automatique entre %d disquettes", len(images))
	}
	return cmd, fifo, nil
}

// disquePropre : les images passées par « The Little Green Desktop » portent un
// secteur d'amorçage qui affiche ce nom sur un écran blanc pendant le chargement.
// On en fait une copie (runtime/disques) dont l'amorce n'est plus exécutée :
// le TOS démarre alors directement le dossier AUTO de la disquette.
type Correctif struct {
	Cherche  string `json:"cherche"`  // octets en hexadécimal
	Remplace string `json:"remplace"` // même longueur
	Note     string `json:"note,omitempty"`
}

// cheminMaxHatari : Hatari (C, tampons de 260 caractères sous Windows) n'ouvre pas un
// chemin complet plus long : au-delà, il reçoit une copie au nom court.
const cheminMaxHatari = 240

// prises : copies déjà données aux autres disquettes du lancement, jamais reprises
// (deux disquettes au contenu identique ont la même empreinte : la 2e renommait la
// copie de la 1re, et Hatari recevait un fichier disparu ; Kick Off 2, A_069).
func disquePropre(p string, correctifs []Correctif, prises map[string]bool) string {
	ext, nomImage := strings.ToLower(filepath.Ext(p)), filepath.Base(p)
	trop := len(p) > cheminMaxHatari
	// Zip (dossier Games de TOSEC copié tel quel) : notre Hatari Windows est compilé
	// sans zlib ; l'image est extraite ici, et les sauvegardes du jeu vont dans la copie.
	dezip := ext == ".zip"
	if !dezip && ext != ".st" && ext != ".msa" && !trop {
		return p
	}
	var d []byte
	var err error
	if dezip {
		d, nomImage, err = imageDuZip(p)
		if err != nil {
			log.Printf("zip illisible (%v) : %s", err, filepath.Base(p))
			return p
		}
		ext = strings.ToLower(filepath.Ext(nomImage))
	} else {
		d, err = os.ReadFile(p)
	}
	if err != nil || len(d) < 512 {
		return p
	}
	lgd := ext == ".st" && cfg.NettoyerAmorce && bytes.Contains(d[:512], []byte("Little Green Desktop"))
	var cor []Correctif
	if ext == ".st" {
		cor = correctifs
	}
	if !lgd && len(cor) == 0 && !trop && !dezip {
		return p
	}
	// Nom de la copie : empreinte du contenu d'origine et des correctifs, pas du
	// chemin (le pack peut changer de dossier sans perdre les sauvegardes faites
	// dans la copie), puis le nom de l'image, raccourci.
	dir := filepath.Join(base, "runtime", "disques")
	_ = os.MkdirAll(dir, 0o755)
	prefixe := prefixeCopie(crc32.ChecksumIEEE(d), int64(len(d)), cor)
	copie := filepath.Join(dir, prefixe+nomCourt(nomImage, 40))
	if existe(copie) {
		return copie
	}
	if a := ancienneCopie(dir, prefixe, nomImage, func() []byte { return d }, prises); a != "" {
		if err := os.Rename(a, copie); err == nil {
			log.Printf("copie reprise avec ses sauvegardes : %s → %s", filepath.Base(a), filepath.Base(copie))
			return copie
		}
	}
	if lgd {
		// somme des mots du secteur 0 différente de $1234 : amorce ignorée par le TOS
		d[511] ^= 0x01
		log.Printf("amorce Little Green Desktop neutralisée : %s", filepath.Base(p))
	}
	for _, c := range cor {
		a, err1 := hex.DecodeString(c.Cherche)
		b, err2 := hex.DecodeString(c.Remplace)
		if err1 != nil || err2 != nil || len(a) != len(b) || len(a) == 0 {
			log.Printf("correctif invalide : %+v", c)
			continue
		}
		n := bytes.Count(d, a)
		d = bytes.ReplaceAll(d, a, b)
		log.Printf("correctif %s (%d fois) : %s", c.Cherche, n, c.Note)
	}
	if err := os.WriteFile(copie, d, 0o644); err != nil {
		return p
	}
	return copie
}

// prefixeCopie : début du nom des copies de runtime/disques (« 1a2b3c4d- »), d'après le
// CRC et la taille de l'image d'origine et les correctifs appliqués. Sert aussi à
// l'export, pour retrouver les copies (et donc les sauvegardes) d'un jeu.
func prefixeCopie(crc uint32, taille int64, cor []Correctif) string {
	empreinte := fmt.Sprintf("%08x|%d", crc, taille)
	for _, c := range cor {
		empreinte += "|" + c.Cherche + ">" + c.Remplace
	}
	h := sha1.Sum([]byte(empreinte))
	return hex.EncodeToString(h[:4]) + "-"
}

// nomCourt : nom de fichier ramené à n caractères au plus, extension gardée.
func nomCourt(nom string, n int) string {
	r, ext := []rune(nom), []rune(filepath.Ext(nom))
	if len(r) <= n || len(ext) >= n {
		return nom
	}
	return string(r[:n-len(ext)]) + string(ext)
}

// ancienneCopie : copie déjà faite pour cette disquette sous un autre nom (image
// renommée : même préfixe ; ancien nommage du lanceur : 8 chiffres hexadécimaux
// calculés sur le chemin complet, puis le nom de l'image). La plus récente.
// Le nom seul ne suffit pas pour l'ancien nommage (il ressemble au nouveau, et deux
// jeux ont parfois une image du même nom, « disk1.st ») : la copie doit avoir le
// contenu de la disquette d'origine, sauvegardes mises à part. original : chargé
// seulement s'il faut comparer. evite : copies à laisser à d'autres disquettes.
func ancienneCopie(dir, prefixe, nom string, original func() []byte, evite map[string]bool) string {
	es, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var d []byte
	charge := false
	meilleure, date := "", time.Time{}
	for _, e := range es {
		n := e.Name()
		memePrefixe := strings.HasPrefix(n, prefixe)
		ancien := !memePrefixe && len(n) == 9+len(nom) && n[8] == '-' && estHex(n[:8]) && strings.EqualFold(n[9:], nom)
		if e.IsDir() || !(memePrefixe || ancien) || evite[filepath.Join(dir, n)] {
			continue
		}
		if ancien {
			if !charge {
				d, charge = original(), true
			}
			if !memeDisquette(filepath.Join(dir, n), d) {
				continue
			}
		}
		if i, err := e.Info(); err == nil && i.ModTime().After(date) {
			meilleure, date = filepath.Join(dir, n), i.ModTime()
		}
	}
	return meilleure
}

// memeDisquette : même taille, et au moins 80 % des secteurs non vides de l'original
// à l'identique (un jeu n'écrit ses sauvegardes que sur quelques secteurs).
func memeDisquette(p string, original []byte) bool {
	c, err := os.ReadFile(p)
	if err != nil || len(original) == 0 || len(c) != len(original) {
		return false
	}
	pleins, egaux := 0, 0
	for o := 0; o+512 <= len(original); o += 512 {
		s := original[o : o+512]
		if bytes.Count(s, s[:1]) == 512 { // secteur vide (0, $E5…) : pareil sur toutes les disquettes
			continue
		}
		pleins++
		if bytes.Equal(s, c[o:o+512]) {
			egaux++
		}
	}
	return pleins > 0 && egaux*10 >= pleins*8
}

func estHex(s string) bool { _, err := hex.DecodeString(s); return err == nil }

// modeJoystick : « manette » si une manette est branchée (ou imposée), sinon « clavier ».
func modeJoystick() string {
	if nombreManettes() > 0 {
		return "manette"
	}
	return "clavier"
}

// nombreManettes : manettes branchées, ou imposé par config.json ("joystick").
func nombreManettes() int {
	switch cfg.Joystick {
	case "clavier":
		return 0
	case "manette":
		if n := compterManettes(); n > 0 {
			return n
		}
		return 1
	}
	return compterManettes()
}

// apiRetirerCapture : range une capture ratée dans medias/<jeu>/_retirees/.
func apiRetirerCapture(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Chemin string `json:"chemin"`
	}
	// chemin nettoyé avant le contrôle : « medias/../config.json » rangeait config.json
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil || !strings.HasPrefix(path.Clean(q.Chemin), "medias/") {
		erreur(w, 400, "chemin invalide")
		return
	}
	p, err := chemin(path.Clean(q.Chemin))
	if err != nil || !existe(p) {
		erreur(w, 404, "capture introuvable")
		return
	}
	dir := filepath.Join(filepath.Dir(p), "_retirees")
	_ = os.MkdirAll(dir, 0o755)
	if err := os.Rename(p, filepath.Join(dir, filepath.Base(p))); err != nil {
		erreur(w, 500, err.Error())
		return
	}
	repondre(w, map[string]bool{"ok": true})
}

// toucheCapture : touche de capture sans modificateur (config.json « touche_capture »), sauf
// une touche de fonction de l'Atari : Hatari la gardait pour lui et le jeu ne la recevait
// plus (F8 des menus de Mortevielle). Reste toujours AltGr+G, le raccourci de Hatari.
func toucheCapture() string {
	t := strings.TrimSpace(cfg.ToucheCapture)
	if regexp.MustCompile(`(?i)^F([1-9]|10)$`).MatchString(t) {
		return ""
	}
	return t
}

// texteCapture : la touche de capture telle que l'interface et l'encart l'annoncent.
func texteCapture() string { return firstNonEmpty(toucheCapture(), "AltGr+G") }

// configHatari : raccourcis et réglages communs (capture, tir clavier).
func configHatari() string {
	t := toucheCapture()
	tir := firstNonEmpty(cfg.TirClavier, "Right Ctrl")
	tir2 := firstNonEmpty(cfg.TirClavier2, "Left Ctrl")
	return "[KeyShortcutsWithoutMod]\nkScreenShot = " + t + "\nkFastForward = F11\nkFullScreen = \n\n" +
		"[Joystick1]\nkUp = Up\nkDown = Down\nkLeft = Left\nkRight = Right\nkFire = " + tir + "\n\n" +
		"[Joystick0]\nkUp = Z\nkDown = S\nkLeft = Q\nkRight = D\nkFire = " + tir2 + "\n"
}

// toucheCompil : presse la touche du menu de compil quand il est affiché.
var pavePC = map[string]int{"0": 0x52, "1": 0x4F, "2": 0x50, "3": 0x51, "4": 0x4B, "5": 0x4C, "6": 0x4D, "7": 0x47, "8": 0x48, "9": 0x49}
var paveST = map[string]int{"0": 0x70, "1": 0x6d, "2": 0x6e, "3": 0x6f, "4": 0x6a, "5": 0x6b, "6": 0x6c, "7": 0x67, "8": 0x68, "9": 0x69}

// sequenceDemarrage : chargement en accéléré (turbo), touches de démarrage
// (menu de compil, intro de crack, trainer…) pressées pendant ce temps, puis
// retour à la vitesse normale.
func sequenceDemarrage(cmd *exec.Cmd, fifo string, j *Jeu, v2 bool) {
	pid := cmd.Process.Pid
	touches := touchesDemarrage(j)
	okTouche := len(touches) > 0 && cfg.ToucheAuto
	var noms []string
	for _, t := range touches {
		noms = append(noms, t.nom)
	}
	touche := strings.Join(noms, " + ")
	actif := func() bool { return jeux.EnJeu() == "jeu" }
	// les touches à tour de rôle, une toutes les « pas », pendant « duree »
	presser := func(duree, pas time.Duration) {
		debut := time.Now()
		for k := 0; time.Since(debut) < duree && actif(); k++ {
			t := touches[k%len(touches)]
			envoyerTouche(fifo, t.st, t.pc, pid)
			time.Sleep(pas)
		}
		// un relâché perdu (le jeu a pris le clavier entre-temps) laissait le TOS répéter
		// la touche sans fin : on relâche encore, à la fin et 3 s plus tard
		relacher := func() {
			for _, t := range touches {
				if actif() {
					relacherTouche(fifo, t.st)
				}
			}
		}
		relacher()
		time.AfterFunc(3*time.Second, relacher)
	}
	delai := j.ToucheDelai
	if v2 && turboMode() == "disque" {
		attendreFenetre(pid)
		if cfg.CarteChargement {
			go carteChargement(3 * time.Second)
		}
		if okTouche { // le menu arrive vite (chargé en accéléré) : touches répétées 5 s
			log.Printf("touches de démarrage : %s", touche)
			time.Sleep(400*time.Millisecond + time.Duration(delai*float64(time.Second)))
			duree := j.ToucheDuree
			if duree <= 0 {
				duree = 5
			}
			presser(time.Duration(duree*float64(time.Second)), 250*time.Millisecond)
		}
		return
	}
	turbo := cfg.TurboChargement
	if turboMode() == "non" {
		turbo = 0
	}
	if !v2 || turbo <= 0 { // vitesse normale : on attend que le menu s'affiche
		if !okTouche {
			return
		}
		if delai <= 0 {
			delai = 8
		}
		time.Sleep(time.Duration(delai * float64(time.Second)))
		log.Printf("touches de démarrage : %s", touche)
		for i := 0; i < 2*len(touches) && actif(); i++ {
			t := touches[i%len(touches)]
			envoyerTouche(fifo, t.st, t.pc, pid)
			time.Sleep(3500 * time.Millisecond / time.Duration(len(touches)))
		}
		return
	}
	attendreFenetre(pid)
	debut := time.Now()
	if okTouche { // le décompactage après le menu prend du temps : turbo plus long
		turbo += 2.5
	}
	fin := debut.Add(time.Duration(turbo * float64(time.Second)))
	if cfg.CarteChargement {
		go carteChargement(time.Until(fin) + 500*time.Millisecond)
	}
	if okTouche {
		log.Printf("touches de démarrage : %s, pressées pendant le turbo", touche)
		time.Sleep(500 * time.Millisecond)
		presser(2500*time.Millisecond, 200*time.Millisecond)
	}
	time.Sleep(time.Until(fin))
	if actif() {
		log.Printf("fin du turbo après %.1f s", time.Since(debut).Seconds())
		finTurbo(fifo, pid)
	}
}

// turboMode : disque (par défaut), debut (ancienne méthode : N s au début) ou non.
func turboMode() string {
	switch cfg.Turbo {
	case "debut", "non":
		return cfg.Turbo
	}
	return "disque"
}

type toucheDemarrage struct {
	nom    string
	st, pc int // scancodes ST et PC
}

// touchesDemarrage : "sequence" du catalogue, sinon l'ancienne forme
// "touche" + "touche_type" (pave par défaut, chiffre, espace).
func touchesDemarrage(j *Jeu) []toucheDemarrage {
	if j.SansTouches {
		return nil
	}
	noms := j.Sequence
	if len(noms) == 0 {
		switch {
		case j.ToucheType == "espace":
			noms = []string{"espace"}
		case j.Touche == "":
			return nil
		case j.ToucheType == "chiffre":
			noms = []string{j.Touche}
		default:
			noms = []string{"pave" + j.Touche}
		}
	}
	var out []toucheDemarrage
	for _, n := range noms {
		if st, pc, ok := codeTouche(n); ok {
			out = append(out, toucheDemarrage{n, st, pc})
		} else {
			log.Printf("%s : touche de démarrage inconnue %q", j.ID, n)
		}
	}
	return out
}

// codeTouche : espace, entree, echap, F1…F10, pave0…pave9 (pavé numérique),
// 0…9 (rangée des chiffres), ou une lettre (position QWERTY ; « N » des
// « Trainer Y/N ? » est au même endroit en AZERTY).
func codeTouche(nom string) (int, int, bool) {
	n := strings.ToLower(strings.TrimSpace(nom))
	switch n {
	case "espace": // écrans d'intro « press space »
		return 0x39, 0x39, true
	case "entree":
		return 0x1C, 0x1C, true
	case "echap":
		return 0x01, 0x01, true
	}
	if strings.HasPrefix(n, "pave") {
		st, ok := paveST[n[4:]]
		return st, pavePC[n[4:]], ok
	}
	if len(n) >= 2 && n[0] == 'f' {
		if k, err := strconv.Atoi(n[1:]); err == nil && k >= 1 && k <= 10 {
			return 0x3A + k, 0x3A + k, true
		}
	}
	if len(n) == 1 { // mêmes scancodes sur ST et PC pour ces touches
		if i := strings.Index("1234567890", n); i >= 0 {
			return 0x02 + i, 0x02 + i, true
		}
		for _, r := range []struct {
			l string
			b int
		}{{"qwertyuiop", 0x10}, {"asdfghjkl", 0x1E}, {"zxcvbnm", 0x2C}} {
			if i := strings.Index(r.l, n); i >= 0 {
				return r.b + i, r.b + i, true
			}
		}
	}
	return 0, 0, false
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

// ---------- gestion du jeu en cours ----------

type Gestionnaire struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	etat    string // libre | jeu | web
	id      string
	titre   string
	debut   time.Time
	demande bool // arrêt demandé (Échap, manette, menu) : pas une erreur
}

func (g *Gestionnaire) Etat() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.etat
	if e == "" {
		e = "libre"
	}
	return map[string]any{"etat": e, "id": g.id, "titre": g.titre}
}

func (g *Gestionnaire) EnJeu() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.etat == "" {
		return "libre"
	}
	return g.etat
}

func (g *Gestionnaire) Arreter() {
	g.mu.Lock()
	cmd := g.cmd
	etat := g.etat
	g.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		g.mu.Lock()
		g.demande = true
		g.mu.Unlock()
		log.Printf("arrêt du jeu en cours")
		// fermeture propre d'abord : Hatari réécrit alors les disquettes
		// modifiées (sauvegardes) ; on force seulement s'il ne répond pas
		fermerDoucement(cmd.Process.Pid)
		for i := 0; i < 40; i++ {
			time.Sleep(100 * time.Millisecond)
			g.mu.Lock()
			fini := g.cmd != cmd
			g.mu.Unlock()
			if fini {
				return
			}
		}
		log.Printf("le jeu ne répond pas : arrêt forcé")
		_ = cmd.Process.Kill()
	} else if etat == "web" {
		diff.Envoyer("quitter", nil)
	}
}

func (g *Gestionnaire) lancerProcessus(id, titre string, cmd *exec.Cmd) error {
	// place réservée avant cmd.Start : deux demandes rapprochées (double appui)
	// lançaient deux Hatari ; pas de jeu non plus par-dessus une démo
	g.mu.Lock()
	switch g.etat {
	case "jeu", "lancement":
		g.mu.Unlock()
		return errors.New("un jeu tourne déjà")
	case "demo":
		g.mu.Unlock()
		return errors.New("une démo tourne : réessayez dans un instant")
	}
	avant := g.etat
	g.etat = "lancement"
	g.mu.Unlock()
	preparerProcessus(cmd)
	log.Printf("lancement : %s %q", cmd.Path, cmd.Args[1:])
	if err := cmd.Start(); err != nil {
		g.mu.Lock()
		g.etat = avant
		g.mu.Unlock()
		return err
	}
	debut := time.Now()
	g.mu.Lock()
	g.cmd, g.etat, g.id, g.titre, g.debut, g.demande = cmd, "jeu", id, titre, debut, false
	g.mu.Unlock()
	diff.Envoyer("etat", g.Etat())
	go premierPlan(cmd.Process.Pid)
	go func() {
		err := cmd.Wait()
		d := time.Since(debut).Round(time.Second)
		log.Printf("fin du jeu %s après %s (%v)", id, d, err)
		g.mu.Lock()
		demande := g.demande
		g.cmd, g.etat, g.id, g.titre = nil, "libre", "", ""
		g.mu.Unlock()
		e := errStr(err)
		if demande {
			e = ""
		}
		fermerCarteJeu()
		ecranApresHatari()
		diff.Envoyer("fin", map[string]any{"id": id, "duree_s": int(d.Seconds()), "erreur": e})
		diff.Envoyer("etat", g.Etat())
		revenirNavigateur()
	}()
	return nil
}

func errStr(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func apiLancer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		erreur(w, 405, "POST attendu")
		return
	}
	var q struct {
		ID       string `json:"id"`
		Variante int    `json:"variante"`
		Mhz16    bool   `json:"mhz16"`
	}
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		erreur(w, 400, err.Error())
		return
	}
	if rangementEnCours() {
		erreur(w, 409, "rangement des disquettes en cours : patientez")
		return
	}
	// avant de préparer quoi que ce soit : commandeHatari réécrit la config de Hatari
	// et (Linux) efface le tube de commandes du jeu en cours
	switch jeux.EnJeu() {
	case "jeu", "lancement":
		erreur(w, 409, "un jeu tourne déjà")
		return
	case "demo":
		erreur(w, 409, "une démo tourne : réessayez dans un instant")
		return
	}
	debut := time.Now()
	log.Printf("demande de lancement : %s (version %d)", q.ID, q.Variante)
	defer func() { log.Printf("demande de lancement traitée en %s", time.Since(debut).Round(time.Millisecond)) }()
	muCatalogue.Lock()
	c, err := lireCatalogue()
	muCatalogue.Unlock()
	if err != nil {
		erreur(w, 500, err.Error())
		return
	}
	j, ok := trouverJeu(c, q.ID)
	if !ok || q.Variante < 0 || q.Variante >= len(j.Variantes) {
		erreur(w, 404, "jeu inconnu")
		return
	}
	v := &j.Variantes[q.Variante]
	switch v.Type {
	case "hatari":
		cmd, fifo, err := commandeHatari(c, j, v, q.Mhz16)
		if err == nil {
			e := jeux.EnJeu()
			libre := e == "libre" || e == "web" // sinon lancerProcessus refuse (et l'écran est déjà à 50 Hz)
			if libre {
				ecranPourHatari(cfg.PleinEcranJeux)
			}
			if err = jeux.lancerProcessus(j.ID, j.Titre, cmd); err != nil && libre {
				ecranApresHatari()
			}
		}
		if err != nil {
			erreur(w, 500, err.Error())
			return
		}
		_, v2 := hatariExe()
		preparerCarteJeu(j)
		fifoCourantMaj(fifo)
		go sequenceDemarrage(cmd, fifo, j, v2)
		repondre(w, map[string]any{"ok": true, "type": "processus", "joystick": modeJoystick()})
	case "natif":
		ligne := v.Linux
		if runtime.GOOS == "windows" {
			ligne = v.Windows
		}
		if len(ligne) == 0 {
			erreur(w, 400, "pas de version native pour ce système")
			return
		}
		exe, err := chemin(ligne[0])
		if err != nil || !existe(exe) {
			erreur(w, 404, "exécutable absent : "+ligne[0])
			return
		}
		var args []string
		for _, a := range ligne[1:] {
			if strings.Contains(a, "{disque}") {
				p, _ := chemin(v.Disque)
				a = strings.ReplaceAll(a, "{disque}", p)
			}
			args = append(args, a)
		}
		cmd := exec.Command(exe, args...)
		cmd.Dir = filepath.Dir(exe)
		if err := jeux.lancerProcessus(j.ID, j.Titre, cmd); err != nil {
			erreur(w, 500, err.Error())
			return
		}
		repondre(w, map[string]any{"ok": true, "type": "processus"})
	case "web":
		jeux.mu.Lock()
		if jeux.etat == "jeu" || jeux.etat == "lancement" || jeux.etat == "demo" {
			jeux.mu.Unlock()
			erreur(w, 409, "un jeu tourne déjà")
			return
		}
		jeux.etat, jeux.id, jeux.titre, jeux.debut = "web", j.ID, j.Titre, time.Now()
		jeux.mu.Unlock()
		diff.Envoyer("etat", jeux.Etat())
		repondre(w, map[string]any{"ok": true, "type": "web", "url": "/" + strings.TrimPrefix(filepath.ToSlash(v.URL), "/")})
	default:
		erreur(w, 400, "type de variante inconnu")
	}
}

func apiArreter(w http.ResponseWriter, r *http.Request) {
	jeux.Arreter()
	repondre(w, map[string]bool{"ok": true})
}

func apiFinWeb(w http.ResponseWriter, r *http.Request) {
	jeux.mu.Lock()
	if jeux.etat == "web" {
		jeux.etat, jeux.id, jeux.titre = "libre", "", ""
	}
	jeux.mu.Unlock()
	diff.Envoyer("etat", jeux.Etat())
	repondre(w, map[string]bool{"ok": true})
}

// quitterJeu : appelé par la surveillance du clavier et de la manette.
func quitterJeu(raison string) {
	switch jeux.EnJeu() {
	case "jeu":
		log.Printf("sortie demandée (%s)", raison)
		jeux.Arreter()
	case "web":
		log.Printf("sortie web demandée (%s)", raison)
		diff.Envoyer("quitter", map[string]string{"raison": raison})
	}
}
