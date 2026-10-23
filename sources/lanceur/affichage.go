package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// Écran des jeux : filtres du Hatari du pack (variables d'environnement lues
// par notre version de Hatari, sans effet sur un autre Hatari).
//   ecran_filtre : net (pixels nets, sharp bilinear) | lisse | entier | hatari
//   ecran_lignes : 0..100, intensité des lignes de balayage
//   ecran_4_3    : proportions d'un écran d'époque
//   ecran_bords  : bords de l'écran ST montrés à tous les jeux (sinon rognés, voir argsAffichage)

func envAffichage() []string {
	f := cfg.EcranFiltre
	switch f {
	case "net", "lisse", "entier":
	default:
		f = "hatari"
	}
	l := cfg.EcranLignes
	if l < 0 {
		l = 0
	}
	if l > 100 {
		l = 100
	}
	a := "0"
	if cfg.Ecran43 {
		a = "1"
	}
	return []string{"HATARI_LUDO_FILTRE=" + f, "HATARI_LUDO_LIGNES=" + strconv.Itoa(l), "HATARI_LUDO_43=" + a}
}

// argsAffichage : options Hatari qui vont avec le filtre et les bords. Hatari double la
// basse résolution en interne (surface 832×552, pixels en blocs de 2×2) : le lissage
// linéaire n'adoucissait alors que la jointure des blocs, presque comme « net ».
// En « lisse », on garde la taille d'origine (416×276 avec les bords) pour lisser
// les vrais pixels ST ; en fenêtre, --zoom 2 garde la même taille de fenêtre
// (il doit précéder --max-width/--max-height, qu'il écrase).
//
// Bords : Hatari montre tout le bord de l'écran du ST (48 points à gauche et à droite,
// 29 lignes en haut, 47 en bas : 416×276 pour une image de 320×200), noir dans la
// plupart des jeux : l'image du jeu ne prenait que 72 % de la hauteur de l'écran
// (Shufflepuck). La taille maximale de Hatari les rogne à 320×212 (6 lignes en haut et
// en bas) : mêmes proportions que 416×276, le format 4:3 reste juste. Les démos et les
// jeux marqués « bords » (overscan) les gardent, comme tous les jeux si ecran_bords.
func argsAffichage(pleinEcran, bords bool) []string {
	bords = bords || cfg.EcranBords
	w, h := 320, 212
	if bords {
		w, h = 416, 276
	}
	if cfg.EcranFiltre != "lisse" {
		if bords {
			return nil
		}
		w, h = 2*w, 2*h // surface doublée par Hatari
	}
	var a []string
	if cfg.EcranFiltre == "lisse" && !pleinEcran {
		a = append(a, "--zoom", "2")
	}
	return append(a, "--max-width", strconv.Itoa(w), "--max-height", strconv.Itoa(h))
}

func etatAffichage() map[string]any {
	return map[string]any{"filtre": cfg.EcranFiltre, "lignes": cfg.EcranLignes, "format43": cfg.Ecran43, "ecran50": cfg.EcranHz > 0, "bords": cfg.EcranBords}
}

// apiAffichage : réglages de l'écran des jeux, enregistrés dans config.json.
func apiAffichage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var q struct {
			Filtre   *string `json:"filtre"`
			Lignes   *int    `json:"lignes"`
			Format43 *bool   `json:"format43"`
			Ecran50  *bool   `json:"ecran50"` // écran à 50 Hz pendant les jeux
			Bords    *bool   `json:"bords"`   // bords de l'écran ST pour tous les jeux
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			erreur(w, 400, err.Error())
			return
		}
		if q.Filtre != nil {
			cfg.EcranFiltre = *q.Filtre
		}
		if q.Lignes != nil {
			cfg.EcranLignes = *q.Lignes
		}
		if q.Format43 != nil {
			cfg.Ecran43 = *q.Format43
		}
		if q.Bords != nil {
			cfg.EcranBords = *q.Bords
		}
		if q.Ecran50 != nil {
			cfg.EcranHz = 0
			if *q.Ecran50 {
				cfg.EcranHz = 50
			}
		}
		if err := majConfig(map[string]any{"ecran_filtre": cfg.EcranFiltre, "ecran_lignes": cfg.EcranLignes, "ecran_4_3": cfg.Ecran43, "ecran_hz": cfg.EcranHz, "ecran_bords": cfg.EcranBords}); err != nil {
			erreur(w, 500, err.Error())
			return
		}
	}
	repondre(w, etatAffichage())
}

// majConfig : change quelques clés de config.json en gardant les autres telles quelles.
func majConfig(cles map[string]any) error {
	p := filepath.Join(base, "config.json")
	m := map[string]any{}
	if b, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(b, &m); err != nil { // ne pas réécrire config.json avec ces quelques clés seulement
			return fmt.Errorf("config.json illisible : %w", err)
		}
	}
	for k, v := range cles {
		m[k] = v
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(out, '\n'), 0o644)
}
