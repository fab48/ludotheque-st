package main

import (
	"log"
	"sync"
	"time"
)

// Fréquence de l'écran pendant Hatari. Un ST français affiche 50 images/s : sur un
// écran à 60 Hz, une image sur cinq reste affichée deux fois, d'où des à-coups
// réguliers dans les défilements. Le lanceur passe l'écran principal à 50 Hz (ou
// 100 Hz) le temps des parties et des démos, puis remet sa fréquence en revenant
// au bac (changement temporaire : Windows la remet aussi si le lanceur s'arrête).
//   ecran_hz : 50 (défaut) | 0 = ne pas toucher à l'écran

var ecranHz struct {
	sync.Mutex
	change bool // fréquence changée par nous, à remettre
	gen    int  // annule un retour programmé
}

// ecranPourHatari : juste avant de lancer Hatari.
func ecranPourHatari(pleinEcran bool) {
	ecranHz.Lock()
	defer ecranHz.Unlock()
	ecranHz.gen++
	if ecranHz.change || cfg.EcranHz <= 0 || !pleinEcran {
		return
	}
	avant, apres, err := passerFrequence(cfg.EcranHz)
	switch {
	case err != nil:
		log.Printf("écran : pas de passage à %d Hz (%v)", cfg.EcranHz, err)
	case apres != avant:
		ecranHz.change = true
		log.Printf("écran : %d Hz → %d Hz pendant le jeu", avant, apres)
	}
}

// ecranApresHatari : à la fin de Hatari. Retour un peu plus tard, pour ne pas
// faire clignoter l'écran entre deux démos ou en relançant aussitôt un jeu.
func ecranApresHatari() {
	ecranHz.Lock()
	defer ecranHz.Unlock()
	if !ecranHz.change {
		return
	}
	ecranHz.gen++
	g := ecranHz.gen
	time.AfterFunc(1500*time.Millisecond, func() {
		ecranHz.Lock()
		defer ecranHz.Unlock()
		if g == ecranHz.gen && ecranHz.change {
			remettreEcranHz()
		}
	})
}

// ecranAvantSortie : à la sortie du lanceur.
func ecranAvantSortie() {
	ecranHz.Lock()
	defer ecranHz.Unlock()
	if ecranHz.change {
		remettreEcranHz()
	}
}

func remettreEcranHz() { // verrou pris
	if err := remettreFrequence(); err != nil {
		log.Printf("écran : retour à la fréquence d'origine impossible (%v)", err)
	} else {
		log.Printf("écran : fréquence d'origine")
	}
	ecranHz.change = false
}
