package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
)

// Changement automatique de disquette : notre Hatari (HATARI_LUDO_DISQUES) lit la
// liste des disquettes du jeu, chacune suivie de ses fichiers et dossiers. Quand le
// jeu cherche un fichier absent de la disquette du lecteur mais présent sur une
// autre, il insère celle-ci (voir sources/hatari/ludotheque-disques.patch).

// listeDisquettes écrit cette liste dans runtime/hatari et renvoie son chemin.
// images : les chemins tels que Hatari les reçoit (--disk-a), dans l'ordre du jeu.
// Vide s'il n'y a qu'une disquette ou qu'aucune n'a de fichiers lisibles.
func listeDisquettes(images []string, vrais []string) string {
	if len(images) < 2 {
		return ""
	}
	var b bytes.Buffer
	b.WriteString("# Ludothèque ST : disquettes du jeu pour le changement automatique\n")
	lisibles := 0
	for i, img := range images {
		b.WriteString("disque " + img + "\n")
		noms := fichiersDisquette(vrais[i])
		if len(noms) > 0 {
			lisibles++
		}
		for _, n := range noms {
			b.WriteString(n + "\n")
		}
	}
	if lisibles == 0 {
		return ""
	}
	p := filepath.Join(base, "runtime", "hatari", "disques.txt")
	if os.WriteFile(p, b.Bytes(), 0o644) != nil {
		return ""
	}
	return p
}

// fichiersDisquette : fichiers et dossiers d'une image .st/.msa/.dim (FAT12 du TOS),
// en majuscules, depuis la racine (« AUTO\A.PRG »). Rien pour un format maison
// (pas de système de fichiers) ou une image .stx.
func fichiersDisquette(p string) []string {
	d, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".msa":
		d = decompacterMSA(d)
	case ".dim":
		if len(d) > 32 {
			d = d[32:]
		}
	case ".st":
	default:
		return nil
	}
	if len(d) < 512 {
		return nil
	}
	le := binary.LittleEndian
	bps, spc := int(le.Uint16(d[11:])), int(d[13])
	res, nfat := int(le.Uint16(d[14:])), int(d[16])
	nroot, spf := int(le.Uint16(d[17:])), int(le.Uint16(d[22:]))
	if bps != 512 || spc == 0 || res == 0 || nfat == 0 || nfat > 2 || nroot == 0 || nroot > 1024 || spf == 0 || spf > 16 {
		return nil
	}
	if (res+nfat*spf)*bps+nroot*32 > len(d) {
		return nil
	}
	fat := d[res*bps : (res+spf)*bps]
	suivant := func(c int) int {
		o := c * 3 / 2
		if o+1 >= len(fat) {
			return 0xfff
		}
		v := int(fat[o]) | int(fat[o+1])<<8
		if c&1 == 1 {
			return v >> 4
		}
		return v & 0xfff
	}
	racine := (res + nfat*spf) * bps
	donnees, amas := racine+nroot*32, spc*bps
	lire := func(c int) []byte { // dossier : sa chaîne d'amas
		var out []byte
		for n := 0; c >= 2 && c < 0xff0 && n < 64; n++ {
			o := donnees + (c-2)*amas
			if o+amas > len(d) {
				break
			}
			out = append(out, d[o:o+amas]...)
			c = suivant(c)
		}
		return out
	}
	// parcourir : nil si une entrée n'a rien d'un répertoire TOS. À la racine, c'est
	// un format maison ; un sous-dossier abîmé (ou piégé par un crack) est seulement
	// laissé de côté.
	total := 0
	var parcourir func(dir []byte, prefixe string, profondeur int) []string
	parcourir = func(dir []byte, prefixe string, profondeur int) []string {
		noms := []string{}
		for i := 0; i+32 <= len(dir) && total < 4000; i += 32 {
			e := dir[i : i+32]
			if e[0] == 0 {
				break
			}
			attr := e[11]
			if e[0] == 0xe5 || attr&0x08 != 0 { // effacé, nom de volume (ou nom long)
				continue
			}
			if !nomTOS(e[:11]) || attr&0xc0 != 0 {
				return nil
			}
			nom := strings.TrimRight(string(e[:8]), " ")
			if ext := strings.TrimRight(string(e[8:11]), " "); ext != "" {
				nom += "." + ext
			}
			if nom == "." || nom == ".." || nom == "" {
				continue
			}
			nom = prefixe + majusculesASCII(nom)
			noms = append(noms, nom)
			total++
			if attr&0x10 != 0 && profondeur < 8 {
				noms = append(noms, parcourir(lire(int(le.Uint16(e[26:]))), nom+`\`, profondeur+1)...)
			}
		}
		return noms
	}
	return parcourir(d[racine:donnees], "", 0)
}

// nomTOS : nom 8+3 d'une entrée de répertoire (« NOM     EXT »), caractères
// permis par le TOS, espaces seulement en fin de nom ou d'extension.
func nomTOS(e []byte) bool {
	for k, partie := range [][]byte{e[:8], e[8:11]} {
		fin := false
		for i, c := range partie {
			switch {
			case c == ' ':
				fin = true
			case fin, c < 0x21 || c > 0x7e, strings.IndexByte(`"*+,/:;<=>?[\]|`, c) >= 0:
				return false
			case c == '.' && !(k == 0 && (i == 0 || i == 1 && partie[0] == '.')):
				return false // « . » et « .. » seulement
			}
		}
		if k == 0 && partie[0] == ' ' {
			return false
		}
	}
	return true
}

// majusculesASCII : comme le toupper de Hatari (lettres ASCII seulement).
func majusculesASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// decompacterMSA : image .msa (pistes compactées par plages « E5 octet nombre »)
// remise à plat comme une .st.
func decompacterMSA(d []byte) []byte {
	be := binary.BigEndian
	if len(d) < 10 || be.Uint16(d) != 0x0e0f {
		return nil
	}
	spt, faces := int(be.Uint16(d[2:])), int(be.Uint16(d[4:]))+1
	debut, fin := int(be.Uint16(d[6:])), int(be.Uint16(d[8:]))
	if spt == 0 || spt > 64 || faces > 2 || fin < debut || fin > 255 {
		return nil
	}
	taille := spt * 512
	out := make([]byte, 0, (fin-debut+1)*faces*taille)
	o := 10
	for t := 0; t < (fin-debut+1)*faces; t++ {
		if o+2 > len(d) {
			break
		}
		n := int(be.Uint16(d[o:]))
		o += 2
		if o+n > len(d) {
			break
		}
		piste := d[o : o+n]
		o += n
		if n == taille {
			out = append(out, piste...)
			continue
		}
		var p []byte
		for i := 0; i < len(piste) && len(p) < taille; {
			if piste[i] == 0xe5 && i+3 < len(piste) {
				for k := int(be.Uint16(piste[i+2:])); k > 0 && len(p) < taille; k-- {
					p = append(p, piste[i+1])
				}
				i += 4
			} else {
				p = append(p, piste[i])
				i++
			}
		}
		out = append(out, p...)
	}
	return out
}
