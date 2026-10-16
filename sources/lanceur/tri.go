package main

// Tri des jeux de la collection TOSEC Atari ST : une seule édition par jeu (les
// doublons écartés), sans les images incompatibles avec la ludothèque. Sert au
// ménage de disques/ (menage.go) et à « --trier <dossier TOSEC> » (liste des jeux à
// ajouter au catalogue, outils/tosec_tri.json).
//
// Incompatible = Falcon/TT seul, écran monochrome (haute résolution), matériel en
// plus (pistolet, cartouche, disque dur), démo jouable, disque de données/scénario/
// éditeur/sauvegarde, extension qui demande le jeu de base, jeu incomplet, dump non
// identifié (ZZZ-UNK), mauvais dump [b] ou virus [v] sans autre édition, zip sans
// image .st/.msa/.stx.
//
// Choix de l'édition : [ST] crackée plutôt que [STX] protégée, pas de [b], [!]
// (dump vérifié), (fr) puis anglais, [one disk], sans trainer [t], sans [a]
// (variante), version la plus récente.

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// imageTosec : une image (zip ou disquette) de la collection, d'après son nom.
type imageTosec struct {
	Nom, Titre, Version, Annee, Editeur string
	Demo                                bool
	Disque, Total                       int
	Couvre                              []int // « Disk B and C » : une image, deux disquettes
	Langue, Machine, Statut             string
	Notes                               []string
	Chemin                              string // relatif (au pack ou à la collection), en « / »
	Source                              string // ST | STX | PD
}

type editionTosec struct {
	Source string
	Images []*imageTosec
}

// titreTrie : un jeu de la collection après le tri.
type titreTrie struct {
	Cle       string
	Nom       string
	Toutes    []*imageTosec
	Choisie   *editionTosec // nil : titre écarté
	Raison    string        // pourquoi écarté
	Catalogue bool
}

var (
	// Titre (ou sous-titre) qui n'est pas un jeu à lancer
	rePasUnJeu = regexp.MustCompile(`(?i)\b(scenery|data ?disk|datadisk|level disk|course disk|save ?game|save disk|` +
		`savegame|editor|construction (kit|set)|mission disk|extension pack|expansion|` +
		`hint|solution|cheat|patch|installer|utility|utilities|map disk|` +
		`one level demo|demo mix|rolling demo|preview)\b`)
	// Extensions qui demandent le jeu de base (sous-titre après « - »)
	reExtension = regexp.MustCompile(`(?i) - .*\b(promised lands|extended levels|final whistle|return to europe|extra time|` +
		`the pay off|last supper|intro disk|grave of king millias)\b`)
	// Notes entre crochets qui rendent l'image inutilisable ici
	notesRejet = []struct {
		rx     *regexp.Regexp
		raison string
	}{
		{regexp.MustCompile(`(?i)falcon only`), "Falcon seulement"},
		{regexp.MustCompile(`(?i)^monochrome$`), "écran monochrome (haute résolution)"},
		{regexp.MustCompile(`(?i)needs a gun`), "pistolet optique"},
		{regexp.MustCompile(`(?i)needs .*cartouche|cartridge`), "cartouche"},
		{regexp.MustCompile(`(?i)hard disk version|^HD( install)?$`), "disque dur"},
		{regexp.MustCompile(`(?i)^data disk$`), "extension (demande le jeu de base)"},
		{regexp.MustCompile(`(?i)not complete|missing|^partial`), "incomplet"}, // sauf « partial french », voir rejetImage
	}
	systemesRejet = map[string]string{"falcon": "Falcon seulement", "tt": "TT seulement", "falcon030": "Falcon seulement"}
	statutsDev    = map[string]bool{"alpha": true, "beta": true, "proto": true, "prototype": true, "preview": true}
	// Fautes de frappe de TOSEC : même jeu sous deux titres
	aliasTitres = map[string]string{"spacequesttheserienencounter": "spacequestthesarienencounter"}

	reNomTosec   = regexp.MustCompile(`^(.*?)(?: \((demo[^)]*)\))? \(([0-9x?]{4}[0-9x?-]*)\)\(([^)]*)\)(.*)$`)
	reVersion    = regexp.MustCompile(`^(.*?) (v\d[\w.]*|V\d[\w.]*|Rev \w+)$`)
	reParens     = regexp.MustCompile(`\(([^)]*)\)`)
	reCrochets   = regexp.MustCompile(`\[([^\]]*)\]`)
	reDiskTosec  = regexp.MustCompile(`(?i)^disk (.+?)(?: of (\d+|\w))?$`)
	reNumDisque  = regexp.MustCompile(`\b(\d+|[A-Za-z])\b`)
	reLangue     = regexp.MustCompile(`^[a-z]{2}(-[a-z]{2})*$`)
	rePays       = regexp.MustCompile(`^[A-Z]{2}(-[A-Z]{2})*$`)
	reMulti      = regexp.MustCompile(`^m\d+$`)
	reLisible    = regexp.MustCompile(`^(.*?), (The|A|An|Le|La|Les|L'|Der|Die|Das)( - .*)?$`)
	reArticle    = regexp.MustCompile(`^(the|a|an|le|la|les|l'|der|die|das) `)
	reNonAlnum   = regexp.MustCompile(`[^a-z0-9]`)
	reSansDisque = regexp.MustCompile(`\(Disk [^)]*\)(\([^)]*\))*`) // « (Disk 1 of 3)(Intro) »
	reSansVar    = regexp.MustCompile(`\[(a\d*|t[^\]]*|!|m[^\]]*)\]`)
	reNombre     = regexp.MustCompile(`\d+(\.\d+)?`)
)

var accents = strings.NewReplacer("à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a", "ç", "c", "é", "e", "è", "e",
	"ê", "e", "ë", "e", "í", "i", "ì", "i", "î", "i", "ï", "i", "ñ", "n", "ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ý", "y", "ÿ", "y", "ß", "ss", "ø", "o", "æ", "ae", "œ", "oe")

// titreLisible : « Bard's Tale, The » → « The Bard's Tale ».
func titreLisible(t string) string {
	if m := reLisible.FindStringSubmatch(t); m != nil {
		sep := " "
		if strings.HasSuffix(m[2], "'") {
			sep = ""
		}
		return m[2] + sep + m[1] + m[3]
	}
	return t
}

// cleTitre : clé de regroupement, sans article, accents, ponctuation ni version.
func cleTitre(t string) string {
	t = accents.Replace(strings.ToLower(titreLisible(t)))
	t = reArticle.ReplaceAllString(t, "")
	t = strings.ReplaceAll(t, "&", "and")
	return reNonAlnum.ReplaceAllString(t, "")
}

// lireNomTosec : « Titre vX (date)(éditeur)(drapeaux)…(Disk 1 of 2)[notes].zip ».
func lireNomTosec(nom string) *imageTosec {
	b := strings.TrimSuffix(nom, filepath.Ext(nom))
	m := reNomTosec.FindStringSubmatch(b)
	if m == nil {
		return nil
	}
	e := &imageTosec{Nom: nom, Titre: m[1], Annee: m[3], Editeur: m[4], Demo: m[2] != ""}
	if len(e.Annee) > 4 {
		e.Annee = e.Annee[:4]
	}
	if v := reVersion.FindStringSubmatch(e.Titre); v != nil {
		e.Titre, e.Version = v[1], v[2]
	}
	reste := m[5]
	for _, c := range reCrochets.FindAllStringSubmatch(reste, -1) {
		e.Notes = append(e.Notes, c[1])
	}
	for _, p := range reParens.FindAllStringSubmatch(reCrochets.ReplaceAllString(reste, ""), -1) {
		p, pl := p[1], strings.ToLower(p[1])
		if d := reDiskTosec.FindStringSubmatch(p); d != nil {
			if e.Disque != 0 { // « (Disk 2 of 4)(Disk A) » : le premier compte
				continue
			}
			for _, x := range reNumDisque.FindAllString(d[1], -1) {
				if n, err := strconv.Atoi(x); err == nil {
					e.Couvre = append(e.Couvre, n)
				} else {
					e.Couvre = append(e.Couvre, int(strings.ToUpper(x)[0]-'A')+1)
				}
			}
			if len(e.Couvre) == 0 {
				e.Couvre = []int{99} // disquette nommée (« Disk Earth ») : jeu jamais complet
			}
			e.Disque = e.Couvre[0]
			e.Total, _ = strconv.Atoi(d[2])
			continue
		}
		switch {
		case pl == "ste" || pl == "stf" || pl == "st" || pl == "mega ste" || pl == "falcon" || pl == "falcon030" || pl == "tt":
			e.Machine = pl
		case pl == "pd" || pl == "sw" || pl == "fw" || pl == "sw-r" || pl == "cw" || pl == "gw":
		case statutsDev[pl] || strings.HasPrefix(pl, "demo"):
			e.Statut = pl
			e.Demo = e.Demo || strings.HasPrefix(pl, "demo")
		case reMulti.MatchString(pl):
			if e.Langue == "" {
				e.Langue = strings.ToUpper(p)
			}
		case reLangue.MatchString(p):
			if e.Langue == "" {
				e.Langue = p
			}
		case rePays.MatchString(p):
			if e.Langue == "" {
				var l []string
				for _, x := range strings.Split(p, "-") {
					switch x {
					case "GB", "US", "AU", "EU":
						l = append(l, "en")
					default:
						l = append(l, strings.ToLower(x))
					}
				}
				e.Langue = strings.Join(l, "-")
			}
		}
	}
	return e
}

// rejetImage : pourquoi cette image ne peut pas servir ("" sinon).
func rejetImage(e *imageTosec) string {
	if r, ok := systemesRejet[e.Machine]; ok {
		return r
	}
	if e.Demo {
		return "démo jouable"
	}
	if strings.HasPrefix(e.Titre, "ZZZ-UNK-") {
		return "non identifié (ZZZ-UNK)"
	}
	if rePasUnJeu.MatchString(e.Titre) {
		return "pas un jeu (données, éditeur…)"
	}
	if reExtension.MatchString(e.Titre) {
		return "extension (demande le jeu de base)"
	}
	for _, n := range e.Notes {
		for _, r := range notesRejet {
			if r.rx.MatchString(n) && !strings.HasPrefix(strings.ToLower(n), "partial french") {
				return r.raison
			}
		}
	}
	return ""
}

// drapeau : notes TOSEC « [b] », « [b2] », « [cr Replicants] »… commençant par cette lettre.
func drapeau(e *imageTosec, lettre string) bool {
	for _, n := range e.Notes {
		if strings.HasPrefix(n, lettre) {
			r := strings.TrimLeft(n[len(lettre):], "0123456789")
			if r == "" || r[0] == ' ' {
				return true
			}
		}
	}
	return false
}

func edAvec(ed *editionTosec, f func(*imageTosec) bool) bool {
	for _, i := range ed.Images {
		if f(i) {
			return true
		}
	}
	return false
}

// scoreEdition : plus c'est haut, mieux c'est.
func scoreEdition(ed *editionTosec) float64 {
	e := ed.Images[0]
	a := func(l string) bool { return edAvec(ed, func(i *imageTosec) bool { return drapeau(i, l) }) }
	s := map[string]float64{"ST": 40, "STX": 25, "PD": 30}[ed.Source]
	if a("b") {
		s -= 100
	}
	if a("v") {
		s -= 200
	}
	if edAvec(ed, func(i *imageTosec) bool {
		for _, n := range i.Notes {
			if n == "!" {
				return true
			}
		}
		return false
	}) {
		s += 6
	}
	if a("cr") {
		s += 8
	}
	if a("t") {
		s -= 6 // trainer : « Y/N ? » au démarrage
	}
	if a("a") {
		s -= 2
	}
	if a("h") || a("o") || a("u") {
		s -= 5
	}
	if a("m") {
		s -= 1 // modifiée (intro, menu du groupe)
	}
	if a("f") {
		s += 1
	}
	if statutsDev[e.Statut] {
		s -= 10
	}
	lg := strings.ToLower(e.Langue)
	switch {
	case contient(strings.Split(lg, "-"), "fr"):
		s += 10
	case lg == "" || lg == "en":
		s += 7
	case strings.HasPrefix(lg, "m"):
		s += 6 // multilingue : souvent avec le français
	}
	if edAvec(ed, func(i *imageTosec) bool {
		for _, n := range i.Notes {
			if strings.Contains(strings.ToLower(n), "one disk") {
				return true
			}
		}
		return false
	}) {
		s += 4
	}
	s -= 2 * float64(len(ed.Images)-1) // moins de disquettes à changer
	if v := reNombre.FindString(e.Version); v != "" {
		f, _ := strconv.ParseFloat(v, 64)
		s += 0.01 * f // à égalité, la version la plus récente
	}
	return s
}

func contient(l []string, x string) bool {
	for _, y := range l {
		if y == x {
			return true
		}
	}
	return false
}

// editionsTosec : regroupe les images d'un même titre en éditions (jeux de
// disquettes complets) ; rend aussi les disquettes de jeux incomplets.
func editionsTosec(images []*imageTosec, source string) ([]*editionTosec, []*imageTosec) {
	sansDisque := func(e *imageTosec, fort bool) string {
		n := reSansDisque.ReplaceAllString(e.Nom, "")
		if fort { // [t] ou [a] parfois sur une seule disquette
			n = reSansVar.ReplaceAllString(n, "")
		}
		return n
	}
	var eds [][]*imageTosec
	var simples, multi []*imageTosec
	for _, e := range images {
		if e.Disque == 0 {
			simples = append(simples, e)
		} else {
			multi = append(multi, e)
		}
	}
	for _, fort := range []bool{false, true} {
		var ordre []string
		groupes := map[string][]*imageTosec{}
		for _, e := range multi {
			k := sansDisque(e, fort)
			if _, ok := groupes[k]; !ok {
				ordre = append(ordre, k)
			}
			groupes[k] = append(groupes[k], e)
		}
		multi = nil
		for _, k := range ordre {
			g := groupes[k]
			tri := append([]*imageTosec(nil), g...)
			sort.SliceStable(tri, func(a, b int) bool { return len(tri[a].Couvre) < len(tri[b].Couvre) })
			parN, total := map[int]*imageTosec{}, 0
			for _, e := range tri {
				for _, n := range e.Couvre {
					if _, ok := parN[n]; !ok {
						parN[n] = e
					}
					if n > total {
						total = n
					}
				}
			}
			for _, e := range g {
				if e.Total > total {
					total = e.Total
				}
			}
			complet := true
			for n := 1; n <= total; n++ {
				if parN[n] == nil {
					complet = false
				}
			}
			if !complet {
				multi = append(multi, g...)
				continue
			}
			var ed []*imageTosec
			for n := 1; n <= total; n++ {
				if !contientImage(ed, parN[n]) {
					ed = append(ed, parN[n])
				}
			}
			eds = append(eds, ed)
		}
	}
	for _, e := range simples {
		eds = append(eds, []*imageTosec{e})
	}
	out := make([]*editionTosec, len(eds))
	for i, ed := range eds {
		out[i] = &editionTosec{Source: source, Images: ed}
	}
	return out, multi
}

func contientImage(l []*imageTosec, e *imageTosec) bool {
	for _, x := range l {
		if x == e {
			return true
		}
	}
	return false
}

// imageLisible : un zip contient bien une image .st/.msa/.stx non vide.
// Réponse gardée tant que le zip ne change pas : le vrac retrie à chaque /api/catalogue.
func imageLisible(p string) bool {
	i, err := os.Stat(p)
	if err != nil || i.Size() == 0 {
		return false
	}
	if !strings.EqualFold(filepath.Ext(p), ".zip") {
		return true
	}
	cle := fmt.Sprintf("%s|%d|%d", p, i.Size(), i.ModTime().UnixNano())
	muLisibles.Lock()
	ok, vu := lisibles[cle]
	muLisibles.Unlock()
	if vu {
		return ok
	}
	ok = zipAvecImage(p)
	muLisibles.Lock()
	lisibles[cle] = ok
	muLisibles.Unlock()
	return ok
}

var (
	muLisibles sync.Mutex
	lisibles   = map[string]bool{}
)

func zipAvecImage(p string) bool {
	z, err := zip.OpenReader(p)
	if err != nil {
		return false
	}
	defer z.Close()
	for _, f := range z.File {
		x := strings.ToLower(filepath.Ext(f.Name))
		if (x == ".st" || x == ".msa" || x == ".stx") && f.UncompressedSize64 > 0 {
			return true
		}
	}
	return false
}

// imageDuZip : la première image .st/.msa/.stx d'un zip, et son nom.
func imageDuZip(p string) ([]byte, string, error) {
	z, err := zip.OpenReader(p)
	if err != nil {
		return nil, "", err
	}
	defer z.Close()
	for _, f := range z.File {
		x := strings.ToLower(filepath.Ext(f.Name))
		if x != ".st" && x != ".msa" && x != ".stx" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, "", err
		}
		d, err := io.ReadAll(r)
		r.Close()
		return d, filepath.Base(filepath.FromSlash(f.Name)), err
	}
	return nil, "", fmt.Errorf("pas d'image .st/.msa/.stx")
}

// trierTosec : images (dans l'ordre des dossiers, puis des noms) → un titre trié
// par jeu. absolu : chemin d'une image sur le disque ; catalogue : clés des titres
// déjà dans la ludothèque.
func trierTosec(images []*imageTosec, absolu func(*imageTosec) string, catalogue map[string]bool) []*titreTrie {
	type parSource struct {
		ordre []string
		imgs  map[string][]*imageTosec
	}
	var ordre []string
	parTitre := map[string]*parSource{}
	ajouter := func(cle string, e *imageTosec) {
		p := parTitre[cle]
		if p == nil {
			p = &parSource{imgs: map[string][]*imageTosec{}}
			parTitre[cle] = p
			ordre = append(ordre, cle)
		}
		if _, ok := p.imgs[e.Source]; !ok {
			p.ordre = append(p.ordre, e.Source)
		}
		p.imgs[e.Source] = append(p.imgs[e.Source], e)
	}
	toutes := func(p *parSource) []*imageTosec {
		var l []*imageTosec
		for _, s := range p.ordre {
			l = append(l, p.imgs[s]...)
		}
		return l
	}
	for _, e := range images {
		c := cleTitre(e.Titre)
		if a, ok := aliasTitres[c]; ok {
			c = a
		}
		ajouter(c, e)
	}
	// « Midwinter II - Flames of Freedom » et « Midwinter II », même éditeur, même année : un seul jeu
	parLongueur := append([]string(nil), ordre...)
	sort.SliceStable(parLongueur, func(a, b int) bool { return len(parLongueur[a]) > len(parLongueur[b]) })
	for _, cle := range parLongueur {
		p := parTitre[cle]
		imgs := toutes(p)
		baseCle := cleTitre(strings.SplitN(imgs[0].Titre, " - ", 2)[0])
		pb := parTitre[baseCle]
		if baseCle == cle || pb == nil {
			continue
		}
		ea := map[string]bool{}
		for _, e := range toutes(pb) {
			ea[e.Editeur+"|"+e.Annee] = true
		}
		commun := false
		for _, e := range imgs {
			commun = commun || ea[e.Editeur+"|"+e.Annee]
		}
		if !commun {
			continue
		}
		delete(parTitre, cle)
		for _, e := range imgs {
			ajouter(baseCle, e)
		}
	}
	var cles []string
	for _, c := range ordre {
		if parTitre[c] != nil {
			cles = append(cles, c)
		}
	}
	sort.Strings(cles)

	var out []*titreTrie
	for _, cle := range cles {
		p := parTitre[cle]
		tout := toutes(p)
		tn := append([]*imageTosec(nil), tout...)
		sort.SliceStable(tn, func(a, b int) bool {
			ba, bb := drapeau(tn[a], "b"), drapeau(tn[b], "b")
			if ba != bb {
				return !ba
			}
			return tn[a].Nom < tn[b].Nom
		})
		t := &titreTrie{Cle: cle, Nom: titreLisible(tn[0].Titre), Toutes: tout, Catalogue: catalogue[cle]}
		var raisons []string
		compte := map[string]int{}
		noter := func(r string, n int, remplacer bool) {
			if _, ok := compte[r]; !ok {
				raisons = append(raisons, r)
			}
			if remplacer {
				compte[r] = n
			} else {
				compte[r] += n
			}
		}
		var eds []*editionTosec
		var orphelines []*imageTosec
		for _, src := range p.ordre {
			var bonnes []*imageTosec
			for _, e := range p.imgs[src] {
				if r := rejetImage(e); r != "" {
					noter(r, 1, false)
				} else {
					bonnes = append(bonnes, e)
				}
			}
			if len(bonnes) == 0 {
				continue
			}
			x, o := editionsTosec(bonnes, src)
			eds = append(eds, x...)
			orphelines = append(orphelines, o...)
		}
		var sain []*editionTosec
		for _, ed := range eds {
			if !edAvec(ed, func(i *imageTosec) bool { return drapeau(i, "v") }) {
				sain = append(sain, ed)
			}
		}
		eds = sain
		sort.SliceStable(eds, func(a, b int) bool { return scoreEdition(eds[a]) > scoreEdition(eds[b]) })
		for _, ed := range eds {
			if scoreEdition(ed) < 0 { // que des mauvais dumps
				break
			}
			ok := true
			for _, i := range ed.Images {
				ok = ok && imageLisible(absolu(i))
			}
			if ok {
				t.Choisie = ed
				break
			}
			noter("zip sans image", 1, false)
		}
		if t.Choisie == nil {
			if len(eds) > 0 {
				noter("mauvais dump [b] seulement", len(eds), true)
			}
			if len(orphelines) > 0 && len(eds) == 0 {
				noter("disquettes manquantes", len(orphelines), true)
			}
			t.Raison = "illisible"
			for _, r := range raisons {
				if t.Raison == "illisible" || compte[r] > compte[t.Raison] {
					t.Raison = r
				}
			}
		} else {
			t.Nom = titreLisible(t.Choisie.Images[0].Titre)
		}
		out = append(out, t)
	}
	return out
}

// sourceTosec : ST, STX ou PD d'après le dossier de la collection (ou l'extension).
func sourceTosec(dossier, nom string) string {
	switch {
	case strings.Contains(dossier, "Public Domain"):
		return "PD"
	case strings.Contains(dossier, "[STX]") || strings.EqualFold(filepath.Ext(nom), ".stx"):
		return "STX"
	}
	return "ST"
}

// trierCollection : « --trier <dossier TOSEC> [--pd] [--tout] [--sortie f.json] ».
// Liste les jeux de Games/ qui ne sont pas encore au catalogue (une édition chacun).
func trierCollection(args []string) error {
	pd, tout := false, false
	sortie := filepath.Join(base, "outils", "tosec_tri.json")
	var rac string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pd":
			pd = true
		case "--tout":
			tout = true
		case "--sortie":
			if i+1 < len(args) {
				sortie, _ = filepath.Abs(args[i+1])
				i++
			}
		default:
			rac = args[i]
		}
	}
	jeuxDir := filepath.Join(rac, "Games")
	dirs, err := os.ReadDir(jeuxDir)
	if err != nil {
		return fmt.Errorf("dossier TOSEC introuvable (il doit contenir Games/) : %w", err)
	}
	var images []*imageTosec
	var illisibles []string
	var noms []string
	for _, d := range dirs {
		noms = append(noms, d.Name())
	}
	sort.Strings(noms)
	for _, d := range noms {
		if !(strings.Contains(d, "[ST]") || strings.Contains(d, "[STX]") || (pd && strings.Contains(d, "Public Domain"))) {
			continue
		}
		fs, _ := os.ReadDir(filepath.Join(jeuxDir, d))
		var fichiers []string
		for _, f := range fs {
			if strings.EqualFold(filepath.Ext(f.Name()), ".zip") {
				fichiers = append(fichiers, f.Name())
			}
		}
		sort.Strings(fichiers)
		for _, f := range fichiers {
			e := lireNomTosec(f)
			if e == nil {
				illisibles = append(illisibles, f)
				continue
			}
			e.Chemin, e.Source = "Games/"+d+"/"+f, sourceTosec(d, f)
			images = append(images, e)
		}
	}
	cat := clesCatalogue()
	titres := trierTosec(images, func(i *imageTosec) string { return filepath.Join(rac, filepath.FromSlash(i.Chemin)) }, cat.tous)

	type jeuSortie struct {
		Titre     string   `json:"titre"`
		Annee     string   `json:"annee"`
		Editeur   string   `json:"editeur"`
		Source    string   `json:"source"`
		Langue    string   `json:"langue"`
		Disques   []string `json:"disques"`
		Notes     []string `json:"notes"`
		Ecartees  int      `json:"ecartees"`
		Version   string   `json:"version,omitempty"`
		Machine   string   `json:"machine,omitempty"`
		Tos       string   `json:"tos,omitempty"`
		Memoire   string   `json:"memoire,omitempty"`
		Protege   bool     `json:"protege,omitempty"` // original Pasti : --fastfdc no conseillé
		Statut    string   `json:"statut,omitempty"`
		Catalogue bool     `json:"catalogue,omitempty"`
	}
	type rejet struct {
		Titre  string `json:"titre"`
		Raison string `json:"raison"`
		Images int    `json:"images"`
	}
	var js []jeuSortie
	var rejets []rejet
	nCat := 0
	reMemoire := regexp.MustCompile(`\d+ ?MB|2Mb`)
	reTos104 := regexp.MustCompile(`(?i)TOS 1\.0\b|don't work with TOS 1\.06`)
	raisons := map[string]int{}
	for _, t := range titres {
		if t.Choisie == nil {
			rejets = append(rejets, rejet{t.Nom, t.Raison, len(t.Toutes)})
			raisons[t.Raison]++
			continue
		}
		if t.Catalogue {
			nCat++
			if !tout {
				continue
			}
		}
		e := t.Choisie.Images[0]
		vues := map[string]bool{}
		notes := []string{}
		for _, i := range t.Choisie.Images {
			for _, n := range i.Notes {
				if !vues[n] {
					vues[n] = true
					notes = append(notes, n)
				}
			}
		}
		sort.Strings(notes)
		j := jeuSortie{Titre: t.Nom, Annee: e.Annee, Editeur: e.Editeur, Source: t.Choisie.Source,
			Langue: firstNonEmpty(e.Langue, "en"), Notes: notes, Ecartees: len(t.Toutes) - len(t.Choisie.Images),
			Version: e.Version, Statut: e.Statut, Catalogue: t.Catalogue, Protege: t.Choisie.Source == "STX"}
		for _, i := range t.Choisie.Images {
			j.Disques = append(j.Disques, i.Chemin)
		}
		for _, n := range notes {
			if strings.ToUpper(n) == "STE" {
				j.Machine = "ste"
			}
			if reTos104.MatchString(n) {
				j.Tos = "tos/tos104fr.img"
			}
			if j.Memoire == "" && reMemoire.MatchString(n) {
				j.Memoire = n
			}
		}
		if e.Machine == "ste" {
			j.Machine = "ste"
		}
		js = append(js, j)
	}
	r := map[string]any{"collection": "TOSEC Atari ST 2012-04-23", "titres": len(titres), "au_catalogue": nCat,
		"jeux": js, "rejets": rejets, "noms_illisibles": illisibles}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // « Tom & Jerry », pas « & »
	enc.SetIndent("", " ")
	_ = enc.Encode(r)
	if err := os.WriteFile(sortie, []byte(b.String()), 0o644); err != nil {
		return err
	}
	log.Printf("tri TOSEC : %d titres, %d déjà au catalogue, %d jeux retenus, %d écartés %v → %s",
		len(titres), nCat, len(js), len(rejets), raisons, sortie)
	return nil
}
