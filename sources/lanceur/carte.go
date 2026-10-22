package main

// Carte des commandes : une fenêtre posée par-dessus le jeu (ou les démos).
//   - pendant le chargement accéléré, en plein écran : elle cache le défilement
//     en accéléré et laisse le temps de lire les commandes ;
//   - en jeu, Pause (clavier) ou Select (manette) l'affiche et met le jeu en
//     pause ; la même touche la referme ;
//   - pour les démos, un bandeau en haut de l'écran.

import (
	"strings"
	"sync"
	"time"
)

type Carte struct {
	Mode   string // plein | centre | haut
	Titre  string
	Lignes []string
}

var (
	muCarte      sync.Mutex
	carteJeu     Carte // commandes du jeu en cours
	carteVisible string
)

func afficherBandeau(l1, l2, l3 string) {
	afficherCarte(Carte{Mode: "haut", Titre: l1, Lignes: []string{l2, l3}})
}

func masquerBandeau() { masquerCarte() }

// lignesCommandes : le même texte que la fiche de l'interface.
func lignesCommandes(j *Jeu) []string {
	n := nombreManettes()
	j1, j2 := "flèches + Ctrl droit", "Z Q S D + Ctrl gauche"
	if n >= 1 {
		j1 = "manette 1"
	}
	if n >= 2 {
		j2 = "manette 2"
	}
	var l []string
	if j.Commandes == "souris" {
		l = append(l, "Souris")
		if j.Joueurs > 1 {
			l = append(l, "Joystick : "+j1)
		}
	} else {
		l = append(l, "Joueur 1 : "+j1)
		if j.Joueurs > 1 {
			l = append(l, "Joueur 2 : "+j2)
		}
	}
	if j.Aide != "" {
		for _, a := range strings.Split(j.Aide, " · ") {
			l = append(l, a)
		}
	}
	l = append(l, "",
		"Échap maintenu 2 s ou Select + Start : retour au bac",
		"Pause ou Select : ces commandes (jeu en pause)  ·  "+texteCapture()+" : capture")
	return l
}

func preparerCarteJeu(j *Jeu) {
	muCarte.Lock()
	carteJeu = Carte{Titre: strings.ToUpper(j.Titre), Lignes: lignesCommandes(j)}
	muCarte.Unlock()
}

// carteChargement : plein écran pendant le turbo, refermée à la fin.
func carteChargement(duree time.Duration) {
	muCarte.Lock()
	c := carteJeu
	carteVisible = "chargement"
	muCarte.Unlock()
	c.Mode = "plein"
	c.Lignes = append([]string{"Chargement…", ""}, c.Lignes...)
	afficherCarte(c)
	time.Sleep(duree)
	muCarte.Lock()
	if carteVisible == "chargement" {
		carteVisible = ""
		muCarte.Unlock()
		masquerCarte()
		return
	}
	muCarte.Unlock()
}

// basculerCarte : Pause ou Select pendant une partie.
func basculerCarte(parManette bool) {
	if jeux.EnJeu() != "jeu" {
		return
	}
	muCarte.Lock()
	c := carteJeu
	ouverte := carteVisible == "jeu"
	if ouverte {
		carteVisible = ""
	} else {
		carteVisible = "jeu"
	}
	muCarte.Unlock()
	if parManette {
		pauseHatari(!ouverte)
	}
	if ouverte {
		masquerCarte()
		revenirJeu()
		return
	}
	c.Mode = "centre"
	c.Lignes = append([]string{"— Pause —", ""}, c.Lignes...)
	afficherCarte(c)
}

func fermerCarteJeu() {
	muCarte.Lock()
	carteVisible = ""
	muCarte.Unlock()
	masquerCarte()
}

// suiviCarte : appelé à chaque tour de la surveillance des entrées.
type suiviTouches struct {
	pause, sel, start bool
	selDepuis         time.Time
	startVu           bool
}

func (s *suiviTouches) maj(pause, sel, start bool) {
	if pause && !s.pause {
		basculerCarte(false)
	}
	if sel && !s.sel {
		s.selDepuis, s.startVu = time.Now(), false
	}
	if sel && start {
		s.startVu = true
	}
	if !sel && s.sel && !s.startVu && time.Since(s.selDepuis) < 700*time.Millisecond {
		basculerCarte(true)
	}
	s.pause, s.sel, s.start = pause, sel, start
}
