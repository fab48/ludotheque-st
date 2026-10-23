package main

// Démos en veille : quand la ludothèque reste inactive, Hatari joue les démos
// célèbres du ST, une par une, avec un bandeau d'explication en haut de l'écran.
// La moindre action (souris, clavier, manette) arrête tout et ramène au bac.

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Demo struct {
	ID      string   `json:"id"`
	Titre   string   `json:"titre"`
	Groupe  string   `json:"groupe,omitempty"`
	Annee   int      `json:"annee,omitempty"`
	Machine string   `json:"machine,omitempty"`
	Tos     string   `json:"tos,omitempty"`
	Texte   string   `json:"texte,omitempty"`
	Cles    []string `json:"cles,omitempty"`    // mots cherchés dans les noms de fichiers de demos/
	Disques []string `json:"disques,omitempty"` // ou chemins exacts
	Options []string `json:"options,omitempty"`
	Dispo   bool     `json:"dispo"`
}

type FichierDemos struct {
	Machine string   `json:"machine"`
	Tos     []string `json:"tos"` // le premier présent est utilisé
	Demos   []Demo   `json:"demos"`
}

// activité de l'utilisateur, mise à jour par la surveillance du clavier, de la souris et des manettes
var derniereActivite atomic.Int64

func signalerActivite() { derniereActivite.Store(time.Now().UnixNano()) }

func lireDemos() (*FichierDemos, []Demo) {
	f := &FichierDemos{Machine: "st"}
	if b, err := os.ReadFile(filepath.Join(base, "demos.json")); err == nil {
		if err := json.Unmarshal(b, f); err != nil {
			log.Printf("demos.json : %v", err)
		}
	}
	// images présentes dans demos/
	var images []string
	_ = filepath.Walk(filepath.Join(base, "demos"), func(p string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() && extImages[strings.ToLower(filepath.Ext(p))] {
			r, _ := filepath.Rel(base, p)
			images = append(images, filepath.ToSlash(r))
		}
		return nil
	})
	prises := map[string]bool{}
	var out []Demo
	for _, d := range f.Demos {
		if len(d.Disques) == 0 {
			for _, im := range images {
				n := strings.ToLower(filepath.Base(im))
				for _, c := range d.Cles {
					if c != "" && strings.Contains(n, strings.ToLower(c)) && !prises[im] {
						d.Disques = append(d.Disques, im)
						prises[im] = true
						break
					}
				}
			}
		}
		d.Dispo = len(d.Disques) > 0
		for _, x := range d.Disques {
			if p, err := chemin(x); err != nil || !existe(p) {
				d.Dispo = false
			}
			prises[x] = true
		}
		out = append(out, d)
	}
	// démos déposées sans fiche
	for _, im := range images {
		if prises[im] {
			continue
		}
		nom := strings.TrimSuffix(filepath.Base(im), filepath.Ext(im))
		out = append(out, Demo{ID: "demo:" + im, Titre: strings.TrimSpace(reEntre.ReplaceAllString(nom, "")),
			Texte: "Démo Atari ST.", Disques: []string{im}, Dispo: true})
	}
	return f, out
}

func apiDemos(w http.ResponseWriter, r *http.Request) {
	_, ds := lireDemos()
	n := 0
	for _, d := range ds {
		if d.Dispo {
			n++
		}
	}
	repondre(w, map[string]any{"demos": ds, "disponibles": n, "apres_min": cfg.DemosApres})
}

var (
	muDemo      sync.Mutex
	demoActive  bool
	demoStop    chan struct{}
	demoLecture bool                     // Play Demo Random : lancé exprès, l'activité n'arrête pas
	demoSuite   = make(chan struct{}, 1) // démo suivante demandée
)

// demoEnLecture : les démos tournent en mode Play Demo Random.
func demoEnLecture() bool {
	muDemo.Lock()
	defer muDemo.Unlock()
	return demoActive && demoLecture
}

// demoSuivante : passe à la démo suivante (Espace ou tir en mode lecture).
func demoSuivante() {
	select {
	case demoSuite <- struct{}{}:
	default:
	}
}

func apiDemoSuivante(w http.ResponseWriter, r *http.Request) {
	demoSuivante()
	repondre(w, map[string]bool{"ok": true})
}

// apiDemoDemarrer : {"mode": "lecture"} pour Play Demo Random, sinon écran de veille.
func apiDemoDemarrer(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Mode string `json:"mode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&q)
	lecture := q.Mode == "lecture"
	f, ds := lireDemos()
	var dispo []Demo
	for _, d := range ds {
		if d.Dispo {
			dispo = append(dispo, d)
		}
	}
	if len(dispo) == 0 {
		erreur(w, 404, "aucune démo dans demos/")
		return
	}
	muDemo.Lock()
	if e := jeux.EnJeu(); e != "libre" && !(e == "demo" && demoActive) {
		muDemo.Unlock()
		erreur(w, 409, "un jeu tourne")
		return
	}
	if demoActive { // veille en cours : Play Demo Random la prend en main
		if lecture {
			demoLecture = true
		}
		muDemo.Unlock()
		repondre(w, map[string]bool{"ok": true})
		return
	}
	demoActive, demoLecture = true, lecture
	demoStop = make(chan struct{})
	stop := demoStop
	muDemo.Unlock()
	select { // une demande « suivante » restée d'avant
	case <-demoSuite:
	default:
	}
	if lecture {
		log.Printf("Play Demo Random : %d démos", len(dispo))
	}
	rand.Shuffle(len(dispo), func(i, j int) { dispo[i], dispo[j] = dispo[j], dispo[i] })
	go boucleDemos(f, dispo, stop)
	repondre(w, map[string]bool{"ok": true})
}

func arreterDemos(raison string) {
	muDemo.Lock()
	defer muDemo.Unlock()
	if demoActive && demoStop != nil {
		log.Printf("démos arrêtées (%s)", raison)
		close(demoStop)
		demoStop = nil
	}
}

func apiDemoArreter(w http.ResponseWriter, r *http.Request) {
	arreterDemos("demande")
	repondre(w, map[string]bool{"ok": true})
}

func tosDemos(f *FichierDemos, d Demo) string {
	if d.Tos != "" {
		return d.Tos
	}
	for _, t := range f.Tos {
		if p, err := chemin(t); err == nil && existe(p) {
			return t
		}
	}
	if c, err := lireCatalogue(); err == nil {
		return c.Hatari.Tos
	}
	return ""
}

func boucleDemos(f *FichierDemos, ds []Demo, stop chan struct{}) {
	defer func() {
		masquerBandeau()
		ecranApresHatari()
		muDemo.Lock()
		demoActive, demoLecture = false, false
		muDemo.Unlock()
		jeux.mu.Lock()
		jeux.etat, jeux.id, jeux.titre = "libre", "", ""
		jeux.mu.Unlock()
		diff.Envoyer("fin-demo", nil)
		diff.Envoyer("etat", jeux.Etat())
		revenirNavigateur()
	}()
	duree := time.Duration(cfg.DureeDemo * float64(time.Minute))
	if duree <= 0 {
		duree = 4 * time.Minute
	}
	for i := 0; ; i++ {
		d := ds[i%len(ds)]
		cmd, _, err := preparerHatari(lancementHatari{
			machine: firstNonEmpty(d.Machine, f.Machine, "st"), tos: tosDemos(f, d), disques: d.Disques,
			options: d.Options, medias: filepath.Join(base, "runtime", "captures-demos"), pleinEcran: true,
			bords: true, // les démos dessinent dans les bords (overscan)
		})
		if err != nil {
			log.Printf("démo %s : %v", d.Titre, err)
			return
		}
		preparerProcessus(cmd)
		ecranPourHatari(true) // sans effet entre deux démos : déjà fait
		if err := cmd.Start(); err != nil {
			log.Printf("démo %s : %v", d.Titre, err)
			return
		}
		log.Printf("démo : %s", d.Titre)
		jeux.mu.Lock()
		jeux.etat, jeux.id, jeux.titre = "demo", d.ID, d.Titre
		jeux.mu.Unlock()
		diff.Envoyer("etat", jeux.Etat())
		fin := make(chan struct{})
		go func() { _ = cmd.Wait(); close(fin) }()
		go premierPlan(cmd.Process.Pid)
		titre := d.Titre
		var info []string
		if d.Groupe != "" {
			info = append(info, d.Groupe)
		}
		if d.Annee > 0 {
			info = append(info, fmt.Sprint(d.Annee))
		}
		if len(info) > 0 {
			titre += " — " + strings.Join(info, ", ")
		}
		if demoEnLecture() {
			afficherBandeau("PLAY DEMO RANDOM : "+titre, d.Texte, "Espace ou tir : démo suivante  ·  Échap ou Select + Start : retour à la ludothèque")
		} else {
			afficherBandeau("DÉMO ATARI ST : "+titre, d.Texte, "Cliquez, bougez la souris ou appuyez sur une touche pour revenir à la ludothèque.")
		}
		debut := time.Now()
		limite := time.After(duree)
		tick := time.NewTicker(100 * time.Millisecond)
		suivante := false
		for !suivante {
			select {
			case <-stop:
				tick.Stop()
				_ = cmd.Process.Kill()
				<-fin
				return
			case <-fin: // la démo s'est arrêtée seule : la suivante
				suivante = true
			case <-limite:
				_ = cmd.Process.Kill()
				<-fin
				suivante = true
			case <-demoSuite:
				if time.Since(debut) < time.Second { // appui resté de la démo d'avant
					continue
				}
				log.Printf("démo suivante demandée")
				_ = cmd.Process.Kill()
				<-fin
				suivante = true
			case <-tick.C:
				// une seconde de grâce après le lancement, puis toute action arrête
				// (sauf en Play Demo Random : Échap, Select + Start ou le menu)
				if !demoEnLecture() && time.Since(debut) > 1500*time.Millisecond && derniereActivite.Load() > debut.Add(time.Second).UnixNano() {
					tick.Stop()
					_ = cmd.Process.Kill()
					<-fin
					log.Printf("démos arrêtées (activité)")
					muDemo.Lock()
					demoStop = nil
					muDemo.Unlock()
					return
				}
			}
		}
		tick.Stop()
	}
}
