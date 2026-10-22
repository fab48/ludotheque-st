package main

// Export : un zip prêt à l'emploi, pour Windows et Linux, avec seulement une partie des
// jeux (les favoris, ou tout le bac). Dézippé n'importe où, il se lance comme le pack :
// Ludotheque.exe ou ./ludotheque.sh. Fichier › Exporter en zip, ou
// « Ludotheque.exe --exporter favoris|bac [--adultes] [--sortie f.zip] ».
//
// Dedans : les deux lanceurs, l'interface, les Hatari, les TOS, les démos, config.json,
// un catalogue.json réduit aux jeux choisis, leurs images, leurs captures, leurs versions
// Reforged et leurs sauvegardes (copies de runtime/disques). Les disquettes du vrac
// rangées dans disques/GAME vont dans disques/vrac/ : le ménage du nouveau pack ne les
// touche pas. Le zip est rangé dans exports/ (jamais dans un export suivant).

import (
	"archive/zip"
	"compress/flate"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var exportation struct {
	mu      sync.Mutex
	enCours bool
	dernier map[string]any
}

// à la racine du pack, s'ils sont là
var fichiersRacine = []string{"Ludotheque.exe", "ludotheque", "ludotheque.sh", "installer-linux.sh",
	"Demarrage automatique - activer.bat", "Demarrage automatique - desactiver.bat",
	"LISEZMOI.txt", "README.md", "config.json", "demos.json",
	"disques/LISEZMOI.txt", "disques/GAME/LISEZMOI.txt"}

// dossiers copiés en entier
var dossiersCommuns = []string{"ui", "tos", "emulateurs", "demos"}

// selectionExport : « favoris » = jeux du catalogue cochés et marqués ★ ; « bac » = tout
// ce que le bac montre (catalogue coché, et le vrac s'il est affiché). Seulement les
// jeux dont une version est là ; les jeux pour adultes avec adultes (Konami code).
func selectionExport(c *Catalogue, mode string, adultes bool) (cat, vr []Jeu) {
	exe, _ := hatariExe()
	garder := func(j *Jeu) bool {
		completer(j, exe != "", c.Hatari.Tos)
		if j.Adulte && !adultes {
			return false
		}
		for _, v := range j.Variantes {
			if v.Dispo {
				return true
			}
		}
		return false
	}
	for _, j := range c.Jeux {
		if j.Actif != nil && !*j.Actif {
			continue
		}
		if mode == "favoris" && (j.Favori == nil || !*j.Favori) {
			continue
		}
		if garder(&j) {
			cat = append(cat, j)
		}
	}
	if mode == "bac" && c.Vrac {
		for _, j := range vrac(c) {
			if garder(&j) {
				vr = append(vr, j)
			}
		}
	}
	return cat, vr
}

// cheminExport : où un fichier du pack va dans le zip (vrac de disques/GAME → disques/vrac).
func cheminExport(rel string) string {
	rel = filepath.ToSlash(rel)
	if p := strings.SplitN(rel, "/", 3); len(p) == 3 && p[0] == "disques" && (strings.EqualFold(p[1], "game") || strings.EqualFold(p[1], "games")) {
		return "disques/vrac/" + p[2]
	}
	return rel
}

// copieSauvegardes : la copie de runtime/disques d'une image, celle que disquePropre
// prendrait (même empreinte, ou copie à l'ancien nom), où le jeu a écrit ses sauvegardes.
func copieSauvegardes(p string, cor []Correctif) string {
	ext, nom := strings.ToLower(filepath.Ext(p)), filepath.Base(p)
	var crc uint32
	var taille int64
	switch ext {
	case ".zip":
		z, err := zip.OpenReader(p)
		if err != nil {
			return ""
		}
		trouve := false
		for _, f := range z.File {
			x := strings.ToLower(filepath.Ext(f.Name))
			if x == ".st" || x == ".msa" || x == ".stx" || x == ".dim" {
				crc, taille, ext, nom, trouve = f.CRC32, int64(f.UncompressedSize64), x, path.Base(f.Name), true
				break
			}
		}
		z.Close()
		if !trouve {
			return ""
		}
	case ".st", ".msa":
		f, err := os.Open(p)
		if err != nil {
			return ""
		}
		h := crc32.NewIEEE()
		taille, _ = io.Copy(h, f)
		f.Close()
		crc = h.Sum32()
	default:
		return ""
	}
	if ext != ".st" {
		cor = nil
	}
	dir := filepath.Join(base, "runtime", "disques")
	prefixe := prefixeCopie(crc, taille, cor)
	if c := filepath.Join(dir, prefixe+nomCourt(nom, 40)); existe(c) {
		return c
	}
	return ancienneCopie(dir, prefixe, nom, func() []byte {
		if strings.EqualFold(filepath.Ext(p), ".zip") {
			d, _, _ := imageDuZip(p)
			return d
		}
		d, _ := os.ReadFile(p)
		return d
	})
}

type listeExport struct {
	fichiers map[string]string // chemin dans le zip → fichier du pack
}

func (l *listeExport) fichier(rel string) {
	if p, err := chemin(rel); err == nil {
		if i, err := os.Stat(p); err == nil && !i.IsDir() {
			l.fichiers[cheminExport(rel)] = p
		}
	}
}

func (l *listeExport) dossier(rel string) {
	racine, err := chemin(rel)
	if err != nil {
		return
	}
	_ = filepath.WalkDir(racine, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		n := strings.ToLower(d.Name())
		if d.IsDir() {
			if n == "_retirees" || n == "_a_supprimer" {
				return filepath.SkipDir
			}
			return nil
		}
		if n == "stderr.txt" || n == "stdout.txt" || strings.HasSuffix(n, ".tmp") {
			return nil
		}
		r, _ := filepath.Rel(base, p)
		l.fichiers[cheminExport(r)] = p
		return nil
	})
}

// fichiersJeu : images, dossiers Reforged, captures et sauvegardes d'un jeu.
func (l *listeExport) jeu(j Jeu) {
	for _, v := range j.Variantes {
		switch v.Type {
		case "hatari":
			for _, d := range v.Disques {
				l.fichier(d)
				if p, err := chemin(d); err == nil {
					if s := copieSauvegardes(p, v.Correctifs); s != "" {
						l.fichiers["runtime/disques/"+filepath.Base(s)] = s
					}
				}
			}
			if v.Tos != "" {
				l.fichier(v.Tos)
			}
			if v.Dossier != "" {
				l.dossier(v.Dossier)
			}
		case "natif": // tout le dossier du programme (données, bibliothèques)
			for _, cmd := range [][]string{v.Windows, v.Linux} {
				if len(cmd) > 0 {
					l.dossier(path.Dir(filepath.ToSlash(cmd[0])))
				}
			}
			if v.Disque != "" {
				l.fichier(v.Disque)
			}
		case "web":
			if v.URL != "" {
				l.dossier(path.Dir(filepath.ToSlash(v.URL)))
			}
		}
	}
	if !j.Vrac {
		l.dossier("medias/" + j.ID)
	}
}

// exporter : écrit le zip ; avance(texte) à chaque étape.
func exporter(mode string, adultes bool, sortie string, avance func(string)) (map[string]any, error) {
	if mode != "favoris" && mode != "bac" {
		return nil, fmt.Errorf("mode inconnu : %q (favoris ou bac)", mode)
	}
	muCatalogue.Lock()
	c, err := lireCatalogue()
	muCatalogue.Unlock()
	if err != nil {
		return nil, err
	}
	avance("Liste des jeux…")
	cat, vr := selectionExport(c, mode, adultes)
	if len(cat)+len(vr) == 0 {
		if mode == "favoris" {
			return nil, errors.New("aucun favori à exporter (★ dans le catalogue, F10)")
		}
		return nil, errors.New("aucun jeu à exporter")
	}

	l := &listeExport{fichiers: map[string]string{}}
	for _, f := range fichiersRacine { // tels quels (disques/GAME/LISEZMOI.txt reste là)
		if p, err := chemin(f); err == nil && existe(p) {
			l.fichiers[f] = p
		}
	}
	for _, d := range dossiersCommuns {
		l.dossier(d)
	}
	avance(fmt.Sprintf("Sauvegardes de %d jeux…", len(cat)+len(vr)))
	for _, j := range cat {
		l.jeu(j)
	}
	for _, j := range vr {
		l.jeu(j)
	}

	// catalogue réduit : les jeux choisis, sans les champs calculés ; le vrac
	// est retrouvé par le nouveau pack dans ses disques/
	nc := *c
	nc.Jeux = nil
	for _, j := range cat {
		j.Captures = nil
		vs := make([]Variante, len(j.Variantes))
		copy(vs, j.Variantes)
		for k := range vs {
			vs[k].Dispo, vs[k].Absent = false, ""
		}
		j.Variantes = vs
		nc.Jeux = append(nc.Jeux, j)
	}
	nc.Vrac = len(vr) > 0
	jsonCat, err := json.MarshalIndent(&nc, "", " ")
	if err != nil {
		return nil, err
	}

	if sortie == "" {
		sortie = filepath.Join(base, "exports", fmt.Sprintf("ludotheque-st-%s-%s.zip", mode, time.Now().Format("20060102-1504")))
	}
	if err := os.MkdirAll(filepath.Dir(sortie), 0o755); err != nil {
		return nil, err
	}
	part := sortie + ".part"
	f, err := os.Create(part)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			_ = os.Remove(part)
		}
	}()
	z := zip.NewWriter(f)
	z.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) { return flate.NewWriter(w, flate.BestSpeed) })
	const dossierZip = "ludotheque-st/"

	noms := make([]string, 0, len(l.fichiers))
	var total int64
	for n, p := range l.fichiers {
		noms = append(noms, n)
		if i, err := os.Stat(p); err == nil {
			total += i.Size()
		}
	}
	sort.Strings(noms)
	var fait int64
	dernier := time.Time{}
	for i, n := range noms {
		if err := ajouterAuZip(z, dossierZip+n, l.fichiers[n]); err != nil {
			return nil, fmt.Errorf("%s : %w", n, err)
		}
		if st, err := os.Stat(l.fichiers[n]); err == nil {
			fait += st.Size()
		}
		if time.Since(dernier) > 500*time.Millisecond {
			dernier = time.Now()
			avance(fmt.Sprintf("Fichier %d sur %d (%d %%)…", i+1, len(noms), fait*100/max(total, 1)))
		}
	}
	w, err := z.CreateHeader(&zip.FileHeader{Name: dossierZip + "catalogue.json", Method: zip.Deflate, Modified: time.Now()})
	if err == nil {
		_, err = w.Write(jsonCat)
	}
	if b := lisiblesExport(l.fichiers); err == nil && b != nil {
		if w, err = z.CreateHeader(&zip.FileHeader{Name: dossierZip + "runtime/lisibles.json", Method: zip.Deflate, Modified: time.Now()}); err == nil {
			_, err = w.Write(b)
		}
	}
	if err == nil {
		err = z.Close()
	}
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		return nil, err
	}
	_ = os.Remove(sortie)
	if err := os.Rename(part, sortie); err != nil {
		return nil, err
	}
	ok = true
	octets := int64(0)
	if i, err := os.Stat(sortie); err == nil {
		octets = i.Size()
	}
	rel, err := filepath.Rel(base, sortie)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = sortie
	}
	log.Printf("export %s : %d jeux du catalogue, %d du vrac, %d fichiers, %d octets → %s", mode, len(cat), len(vr), len(noms)+1, octets, rel)
	return map[string]any{"mode": mode, "jeux": len(cat), "vrac": len(vr), "fichiers": len(noms) + 1,
		"octets": octets, "zip": filepath.ToSlash(rel)}, nil
}

// ajouterAuZip : un fichier du pack, avec sa date ; exécutable (755) pour les
// programmes Linux (sans extension) et les scripts .sh.
func ajouterAuZip(z *zip.Writer, nom, p string) error {
	src, err := os.Open(p)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	h := &zip.FileHeader{Name: nom, Method: zip.Deflate, Modified: info.ModTime()}
	switch strings.ToLower(path.Ext(nom)) {
	case ".zip", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".woff2", ".7z", ".gz":
		h.Method = zip.Store // déjà compressés
	}
	mode := os.FileMode(0o644)
	if x := path.Ext(nom); x == "" || x == ".sh" {
		mode = 0o755
	}
	h.SetMode(mode)
	w, err := z.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

func etatExport() map[string]any {
	exportation.mu.Lock()
	defer exportation.mu.Unlock()
	return map[string]any{"en_cours": exportation.enCours, "dernier": exportation.dernier}
}

// apiExporter : GET = état ; POST {"mode": "favoris"|"bac", "adultes": bool} = exporter,
// en arrière-plan (évènements SSE « export » : étape, puis compte rendu ou erreur).
func apiExporter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		repondre(w, etatExport())
		return
	}
	if r.Method != http.MethodPost {
		erreur(w, 405, "GET ou POST attendu")
		return
	}
	var q struct {
		Mode    string `json:"mode"`
		Adultes bool   `json:"adultes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		erreur(w, 400, err.Error())
		return
	}
	if q.Mode != "favoris" && q.Mode != "bac" {
		erreur(w, 400, "mode : favoris ou bac")
		return
	}
	if rangementEnCours() { // le ménage efface des disquettes que l'export lirait
		erreur(w, 409, "rangement des disquettes en cours : patientez")
		return
	}
	exportation.mu.Lock()
	if exportation.enCours {
		exportation.mu.Unlock()
		erreur(w, 409, "un export est déjà en cours")
		return
	}
	exportation.enCours = true
	exportation.mu.Unlock()
	go func() {
		res, err := exporter(q.Mode, q.Adultes, "", func(t string) {
			diff.Envoyer("export", map[string]any{"en_cours": true, "texte": t})
		})
		if err != nil {
			log.Printf("export %s : %v", q.Mode, err)
			res = map[string]any{"erreur": err.Error()}
		}
		exportation.mu.Lock()
		exportation.enCours, exportation.dernier = false, res
		exportation.mu.Unlock()
		diff.Envoyer("export", map[string]any{"en_cours": false, "dernier": res})
	}()
	repondre(w, map[string]bool{"ok": true})
}

// exporterLigneDeCommande : Ludotheque.exe --exporter favoris|bac [--adultes] [--sortie f.zip]
func exporterLigneDeCommande(args []string) error {
	fl := flag.NewFlagSet("exporter", flag.ContinueOnError)
	adultes := fl.Bool("adultes", false, "avec les jeux pour adultes")
	sortie := fl.String("sortie", "", "zip à écrire (sinon exports/)")
	mode := args[0]
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	if *sortie != "" {
		if a, err := filepath.Abs(*sortie); err == nil {
			*sortie = a
		}
	}
	cfg = chargerConfig()
	res, err := exporter(mode, *adultes, *sortie, func(t string) { fmt.Fprintln(os.Stderr, t) })
	if err != nil {
		return err
	}
	fmt.Printf("%s : %v jeux du catalogue, %v du vrac, %v fichiers, %.1f Mo\n", res["zip"], res["jeux"], res["vrac"], res["fichiers"],
		float64(res["octets"].(int64))/1e6)
	return nil
}
