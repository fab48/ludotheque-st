package main

// Rangement de disques/GAME : l'utilisateur y dépose la collection TOSEC Atari ST,
// telle quelle ou en vrac (tout, ou seulement le dossier Games). Au démarrage (et par
// Fichier › Ménage des disquettes), sans rien demander :
//  1. les images du catalogue qui manquent sont installées depuis ce dépôt
//     (outils/images_tosec.json : zip, membre, CRC32, taille ; par nom, sinon par CRC) ;
//  2. le ménage : une édition gardée par jeu (tri.go), tout le reste du dépôt effacé ;
//  3. les jeux gardés vont au bac (mode vrac).
// Chaque action et sa raison vont dans runtime/menage-<date>.txt.
//
// Jamais touché : ce qui est hors de disques/GAME, un fichier au nom inconnu (pas un
// nom TOSEC), la source d'une image du catalogue qui n'a pas pu être installée.
// Rien n'est refait tant que le contenu du dépôt ne change pas (runtime/rangement.json).

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var rangement struct {
	mu      sync.Mutex
	enCours bool
	texte   string         // étape en cours
	dernier map[string]any // compte rendu du dernier rangement
}

func rangementEnCours() bool {
	rangement.mu.Lock()
	defer rangement.mu.Unlock()
	return rangement.enCours
}

func etatRangement() map[string]any {
	rangement.mu.Lock()
	defer rangement.mu.Unlock()
	return map[string]any{"en_cours": rangement.enCours, "texte": rangement.texte, "dernier": rangement.dernier}
}

func etapeRangement(t string) {
	rangement.mu.Lock()
	rangement.texte = t
	rangement.mu.Unlock()
	diff.Envoyer("rangement", map[string]any{"en_cours": true, "texte": t})
}

// dossiersDepot : disques/GAME, et disques/Games (le dossier de TOSEC copié tel quel),
// GAMES… : tout dossier de disques/ qui porte ce nom, à la casse près.
func dossiersDepot() []string {
	var l []string
	es, _ := os.ReadDir(filepath.Join(base, "disques"))
	for _, e := range es {
		if e.IsDir() && (strings.EqualFold(e.Name(), "game") || strings.EqualFold(e.Name(), "games")) {
			l = append(l, filepath.Join(base, "disques", e.Name()))
		}
	}
	return l
}

// categorieTosec : "jeux" dans un dossier « Atari ST - Games - [ST] (…) », sinon le
// nom d'un autre dossier de la collection (« Demos », « Compilations », « TOS »…),
// "" hors de ses dossiers (images posées en vrac). Rend aussi le dossier.
func categorieTosec(rel string) (string, string) {
	dossiers := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")
	for i, d := range dossiers {
		if i == 0 && len(dossiers) > 1 && (strings.EqualFold(d, "game") || strings.EqualFold(d, "games")) {
			continue // disques/GAME lui-même
		}
		if autresTosec[strings.ToLower(d)] { // « Atari ST [TOSEC]/Compilations/… »
			return d, d
		}
		if strings.HasPrefix(d, "Atari ST - Games - ") {
			return d, "jeux"
		}
		if strings.HasPrefix(d, "Atari ST - ") {
			c := strings.TrimPrefix(d, "Atari ST - ")
			if i := strings.IndexAny(c, "-(["); i > 0 {
				c = c[:i]
			}
			return d, strings.TrimSpace(c)
		}
	}
	return "", ""
}

// Dossiers de premier niveau de la collection qui ne sont pas des jeux.
var autresTosec = map[string]bool{"applications": true, "collections": true, "compilations": true, "coverdisks": true,
	"demos": true, "diskmags": true, "docs": true, "educational": true, "online magazines": true, "tos": true}

// sourceFichier : ST ou STX d'après le contenu (image en vrac, sans dossier de la
// collection pour le dire) ; "" si ce n'est pas une disquette (zip d'un TOS…).
func sourceFichier(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".stx":
		return "STX"
	case ".st", ".msa":
		return "ST"
	}
	z, err := zip.OpenReader(p)
	if err != nil {
		return ""
	}
	defer z.Close()
	for _, f := range z.File {
		switch strings.ToLower(filepath.Ext(f.Name)) {
		case ".stx":
			return "STX"
		case ".st", ".msa":
			return "ST"
		}
	}
	return ""
}

type etatCatalogue struct {
	tous     map[string]bool // titres de la ludothèque (catalogue + sources TOSEC)
	complets map[string]bool // … dont les images sont installées
	proteges map[string]bool // zips (nom en minuscules) d'images encore absentes
	absents  int             // jeux du catalogue sans leurs images
}

// clesCatalogue : les jeux de la ludothèque, vus par leur titre TOSEC.
func clesCatalogue() etatCatalogue {
	e := etatCatalogue{tous: map[string]bool{}, complets: map[string]bool{}, proteges: map[string]bool{}}
	present := func(rel string) bool { p, err := chemin(rel); return err == nil && existe(p) }
	if c, err := lireCatalogue(); err == nil {
		for _, j := range c.Jeux {
			k := cleTitre(j.Titre)
			e.tous[k] = true
			complet := false
			for _, v := range j.Variantes {
				if v.Type != "hatari" || len(v.Disques) == 0 {
					continue
				}
				ok := true
				for _, d := range v.Disques {
					ok = ok && present(d)
				}
				complet = complet || ok
			}
			if complet {
				e.complets[k] = true
			} else if j.Actif == nil || *j.Actif {
				e.absents++
			}
		}
	}
	parCle := map[string]bool{} // clé → toutes ses images installées
	for dest, src := range lireImagesTosec() {
		nom := filepath.Base(filepath.FromSlash(src.Zip))
		ok := present(dest)
		if !ok {
			e.proteges[strings.ToLower(nom)] = true
		}
		if !strings.Contains(src.Zip, "Games/") {
			continue
		}
		if x := lireNomTosec(nom); x != nil {
			k := cleTitre(x.Titre)
			e.tous[k] = true
			if v, vu := parCle[k]; vu {
				parCle[k] = v && ok
			} else {
				parCle[k] = ok
			}
		}
	}
	for k, ok := range parCle {
		if ok {
			e.complets[k] = true
		} else {
			delete(e.complets, k)
		}
	}
	return e
}

type sourceImage struct {
	Zip    string `json:"zip"`
	Membre string `json:"membre"`
	Crc32  string `json:"crc32"`
	Taille int64  `json:"taille"`
}

func lireImagesTosec() map[string]sourceImage {
	var m struct {
		Images map[string]sourceImage `json:"images"`
	}
	b, _ := os.ReadFile(filepath.Join(base, "outils", "images_tosec.json"))
	_ = json.Unmarshal(b, &m)
	return m.Images
}

// ---------- 1. installation du catalogue ----------

type journalRangement struct{ lignes []string }

func (j *journalRangement) noter(action, raison, quoi string) {
	j.lignes = append(j.lignes, action+"\t"+raison+"\t"+quoi)
}

// installerCatalogue : les images du catalogue absentes, prises dans le dépôt.
func installerCatalogue(depot string, fichiers []string, jr *journalRangement) (int, []string) {
	var manquantes []string
	images := lireImagesTosec()
	for dest := range images {
		if p, err := chemin(dest); err == nil && !existe(p) {
			manquantes = append(manquantes, dest)
		}
	}
	sort.Strings(manquantes)
	if len(manquantes) == 0 {
		return 0, nil
	}
	parNom := map[string][]string{}
	for _, f := range fichiers {
		k := strings.ToLower(filepath.Base(f))
		parNom[k] = append(parNom[k], f)
	}
	ecrire := func(dest string, d []byte, depuis string) bool {
		src := images[dest]
		if fmt.Sprintf("%08x", crc32.ChecksumIEEE(d)) != src.Crc32 || int64(len(d)) != src.Taille {
			return false
		}
		p, err := chemin(dest)
		if err != nil {
			return false
		}
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if os.WriteFile(p, d, 0o644) != nil {
			return false
		}
		r, _ := filepath.Rel(base, filepath.Join(depot, depuis))
		jr.noter("INSTALLÉ", "image du catalogue", dest+"  ←  "+filepath.ToSlash(r))
		return true
	}
	lireMembre := func(p string, garder func(*zip.File) bool) []byte {
		z, err := zip.OpenReader(p)
		if err != nil {
			return nil
		}
		defer z.Close()
		for _, f := range z.File {
			if !garder(f) {
				continue
			}
			r, err := f.Open()
			if err != nil {
				return nil
			}
			d, _ := io.ReadAll(r)
			r.Close()
			return d
		}
		return nil
	}
	n := 0
	var reste []string
	for i, dest := range manquantes {
		etapeRangement(fmt.Sprintf("Installation du catalogue : %d / %d", i+1, len(manquantes)))
		src := images[dest]
		ok := false
		// par le nom du zip, ou de l'image déjà décompressée
		for _, f := range parNom[strings.ToLower(filepath.Base(filepath.FromSlash(src.Zip)))] {
			d := lireMembre(filepath.Join(depot, f), func(z *zip.File) bool {
				return z.Name == src.Membre || (fmt.Sprintf("%08x", z.CRC32) == src.Crc32 && int64(z.UncompressedSize64) == src.Taille)
			})
			if ok = d != nil && ecrire(dest, d, f); ok {
				break
			}
		}
		for _, f := range parNom[strings.ToLower(filepath.Base(filepath.FromSlash(src.Membre)))] {
			if ok {
				break
			}
			if d, err := os.ReadFile(filepath.Join(depot, f)); err == nil {
				ok = ecrire(dest, d, f)
			}
		}
		if ok {
			n++
		} else {
			reste = append(reste, dest)
		}
	}
	// dernier recours : le contenu (CRC) dans tous les zips du dépôt
	if len(reste) > 0 {
		voulu := map[string]string{}
		for _, dest := range reste {
			voulu[fmt.Sprintf("%s|%d", images[dest].Crc32, images[dest].Taille)] = dest
		}
		for i, f := range fichiers {
			if len(voulu) == 0 {
				break
			}
			if !strings.EqualFold(filepath.Ext(f), ".zip") {
				continue
			}
			if i%500 == 0 {
				etapeRangement(fmt.Sprintf("Recherche de %d images du catalogue : %d / %d", len(voulu), i, len(fichiers)))
			}
			z, err := zip.OpenReader(filepath.Join(depot, f))
			if err != nil {
				continue
			}
			var trouves []*zip.File
			for _, zf := range z.File {
				if _, ok := voulu[fmt.Sprintf("%08x|%d", zf.CRC32, zf.UncompressedSize64)]; ok {
					trouves = append(trouves, zf)
				}
			}
			for _, zf := range trouves {
				k := fmt.Sprintf("%08x|%d", zf.CRC32, zf.UncompressedSize64)
				r, err := zf.Open()
				if err != nil {
					continue
				}
				d, _ := io.ReadAll(r)
				r.Close()
				if ecrire(voulu[k], d, f) {
					n++
					delete(voulu, k)
				}
			}
			z.Close()
		}
		reste = reste[:0]
		for _, dest := range voulu {
			reste = append(reste, dest)
		}
		sort.Strings(reste)
	}
	for _, dest := range reste {
		jr.noter("ABSENT", "image du catalogue introuvable dans le dépôt", dest)
	}
	return n, reste
}

// ---------- 2. ménage ----------

type fichierMenage struct {
	Chemin string // relatif au dépôt
	Raison string
	Octets int64
}

// trierDepot : ce qu'on garde et ce qu'on efface dans le dépôt.
func trierDepot(depot string, fichiers []string, jr *journalRangement) (int, []fichierMenage) {
	cat := clesCatalogue()
	var images []*imageTosec
	var effacer []fichierMenage
	taille := func(f string) int64 {
		if st, err := os.Stat(filepath.Join(depot, f)); err == nil {
			return st.Size()
		}
		return 0
	}
	ajouter := func(f, raison string) {
		if cat.proteges[strings.ToLower(filepath.Base(f))] {
			jr.noter("GARDÉ", "source d'une image du catalogue pas encore installée", f)
			return
		}
		effacer = append(effacer, fichierMenage{f, raison, taille(f)})
	}
	for i, f := range fichiers {
		if i%2000 == 0 {
			etapeRangement(fmt.Sprintf("Tri des disquettes : %d / %d", i, len(fichiers)))
		}
		nom := filepath.Base(f)
		if strings.EqualFold(nom, "LISEZMOI.txt") {
			continue
		}
		dt, categorie := categorieTosec(f)
		e := lireNomTosec(nom)
		if e == nil {
			if categorie != "" && categorie != "jeux" {
				ajouter(f, "pas un jeu ("+categorie+")")
			} else {
				jr.noter("GARDÉ", "nom inconnu (pas un nom TOSEC)", f)
			}
			continue
		}
		x := strings.ToLower(filepath.Ext(nom))
		if categorie != "" && categorie != "jeux" {
			ajouter(f, "pas un jeu ("+categorie+")")
			continue
		}
		if x != ".zip" && x != ".st" && x != ".msa" && x != ".stx" {
			ajouter(f, "pas une disquette")
			continue
		}
		src := sourceTosec(dt, nom)
		if categorie == "" { // en vrac : le dossier ne dit pas [ST] ou [STX]
			if src = sourceFichier(filepath.Join(depot, f)); src == "" {
				ajouter(f, "pas une disquette (TOS, archive…)")
				continue
			}
		}
		e.Chemin, e.Source = f, src
		images = append(images, e)
	}
	etapeRangement("Choix d'une édition par jeu…")
	sort.SliceStable(images, func(a, b int) bool { return images[a].Chemin < images[b].Chemin })
	titres := trierTosec(images, func(i *imageTosec) string { return filepath.Join(depot, filepath.FromSlash(i.Chemin)) }, cat.complets)
	gardes := 0
	for _, t := range titres {
		garder := map[*imageTosec]bool{}
		if t.Choisie != nil && !t.Catalogue {
			gardes++
			for _, i := range t.Choisie.Images {
				garder[i] = true
				jr.noter("GARDÉ", "meilleure édition de « "+t.Nom+" »", i.Chemin)
			}
		}
		for _, i := range t.Toutes {
			if garder[i] {
				continue
			}
			r := rejetImage(i)
			switch {
			case r != "":
			case t.Choisie == nil:
				r = t.Raison
			case t.Catalogue:
				r = "déjà au catalogue"
			default:
				r = "doublon (autre édition)"
			}
			ajouter(i.Chemin, r)
		}
	}
	return gardes, effacer
}

// signatureDepot : change dès qu'un fichier du dépôt est ajouté, retiré ou modifié.
// Les fichiers sont rendus relatifs à disques/ (« GAME/… »).
func signatureDepot(depot string) (string, []string) {
	var fichiers []string
	var n, octets, recent int64
	for _, dd := range dossiersDepot() {
		_ = filepath.WalkDir(dd, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(depot, p)
			fichiers = append(fichiers, filepath.ToSlash(rel))
			if i, err := d.Info(); err == nil {
				n++
				octets += i.Size()
				if t := i.ModTime().Unix(); t > recent {
					recent = t
				}
			}
			return nil
		})
	}
	sort.Strings(fichiers)
	return fmt.Sprintf("%d|%d|%d", n, octets, recent), fichiers
}

// lancerRangement : en arrière-plan ; force = même si le dépôt n'a pas changé.
func lancerRangement(force bool) bool {
	rangement.mu.Lock()
	if rangement.enCours {
		rangement.mu.Unlock()
		return false
	}
	rangement.enCours = true
	rangement.mu.Unlock()
	go func() {
		res := ranger(force)
		rangement.mu.Lock()
		rangement.enCours, rangement.texte = false, ""
		if res != nil {
			rangement.dernier = res
		}
		rangement.mu.Unlock()
		diff.Envoyer("rangement", map[string]any{"en_cours": false, "dernier": res})
	}()
	return true
}

func ranger(force bool) map[string]any {
	depot := filepath.Join(base, "disques") // les chemins des fichiers commencent par GAME/
	sig, fichiers := signatureDepot(depot)
	etatF := filepath.Join(base, "runtime", "rangement.json")
	var avant struct {
		Signature string `json:"signature"`
	}
	b, _ := os.ReadFile(etatF)
	_ = json.Unmarshal(b, &avant)
	vide := len(fichiers) == 0 || (len(fichiers) == 1 && strings.EqualFold(filepath.Base(fichiers[0]), "LISEZMOI.txt"))
	if vide || (!force && sig == avant.Signature) {
		if force {
			return map[string]any{"vide": vide, "fichiers": len(fichiers), "depot": "disques/GAME"}
		}
		return nil
	}
	if jeux.EnJeu() != "libre" {
		log.Printf("rangement remis à plus tard : un jeu tourne")
		return nil
	}
	debut := time.Now()
	log.Printf("rangement de disques/GAME : %d fichiers", len(fichiers))
	jr := &journalRangement{}

	installees, absentes := installerCatalogue(depot, fichiers, jr)
	gardes, effacer := trierDepot(depot, fichiers, jr)

	etapeRangement(fmt.Sprintf("Ménage : %d fichiers inutiles…", len(effacer)))
	type ligne struct {
		Raison   string `json:"raison"`
		Fichiers int    `json:"fichiers"`
		Octets   int64  `json:"octets"`
	}
	parRaison := map[string]*ligne{}
	var octets int64
	effaces, echecs := 0, 0
	for _, f := range effacer {
		if err := os.Remove(filepath.Join(depot, filepath.FromSlash(f.Chemin))); err != nil && !os.IsNotExist(err) {
			echecs++
			jr.noter("ÉCHEC", err.Error(), f.Chemin)
			continue
		}
		effaces++
		octets += f.Octets
		jr.noter("EFFACÉ", f.Raison, f.Chemin)
		l := parRaison[f.Raison]
		if l == nil {
			l = &ligne{Raison: f.Raison}
			parRaison[f.Raison] = l
		}
		l.Fichiers++
		l.Octets += f.Octets
	}
	// dossiers vidés par le ménage (disques/GAME lui-même reste)
	var dossiers []string
	for _, dd := range dossiersDepot() {
		_ = filepath.WalkDir(dd, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && p != dd {
				dossiers = append(dossiers, p)
			}
			return nil
		})
	}
	for i := len(dossiers) - 1; i >= 0; i-- {
		_ = os.Remove(dossiers[i]) // échoue s'il n'est pas vide
	}
	var lignes []*ligne
	for _, l := range parRaison {
		lignes = append(lignes, l)
	}
	sort.Slice(lignes, func(a, b int) bool {
		if lignes[a].Fichiers != lignes[b].Fichiers {
			return lignes[a].Fichiers > lignes[b].Fichiers
		}
		return lignes[a].Raison < lignes[b].Raison
	})

	// les jeux gardés vont au bac
	if gardes > 0 {
		muCatalogue.Lock()
		if c, err := lireCatalogue(); err == nil && !c.Vrac {
			c.Vrac = true
			if ecrireCatalogue(c) == nil {
				jr.noter("RÉGLÉ", "jeux gardés mis dans le bac", "mode vrac activé")
			}
		}
		muCatalogue.Unlock()
	}

	nom := "menage-" + time.Now().Format("20060102-150405") + ".txt"
	entete := []string{
		"Ludothèque ST — rangement de disques/GAME du " + time.Now().Format("02/01/2006 15:04"),
		fmt.Sprintf("%d fichiers lus, %d images du catalogue installées, %d jeux gardés, %d fichiers effacés (%d octets), %d échecs.",
			len(fichiers), installees, gardes, effaces, octets, echecs),
		"Colonnes : action, raison, fichier (chemin dans disques/).", "",
	}
	_ = os.WriteFile(filepath.Join(base, "runtime", nom), []byte(strings.Join(append(entete, jr.lignes...), "\n")+"\n"), 0o644)
	sig, _ = signatureDepot(depot)
	b, _ = json.Marshal(map[string]string{"signature": sig})
	_ = os.WriteFile(etatF, b, 0o644)
	log.Printf("rangement : %d images du catalogue installées, %d jeux gardés, %d fichiers effacés (%d octets), %d échecs, en %v → runtime/%s",
		installees, gardes, effaces, octets, echecs, time.Since(debut).Round(time.Second), nom)
	return map[string]any{
		"depot": "disques/GAME", "fichiers": len(fichiers), "installees": installees,
		"absentes": len(absentes), "jeux": gardes, "effaces": effaces, "octets": octets, "echecs": echecs,
		"raisons": lignes, "journal": "runtime/" + nom,
	}
}

// apiMenage : GET = état du rangement ; POST = le refaire maintenant.
func apiMenage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		repondre(w, etatRangement())
	case http.MethodPost:
		if jeux.EnJeu() != "libre" {
			erreur(w, 409, "un jeu tourne : quittez-le d'abord")
			return
		}
		lancerRangement(true)
		repondre(w, etatRangement())
	default:
		erreur(w, 405, "GET ou POST attendu")
	}
}
