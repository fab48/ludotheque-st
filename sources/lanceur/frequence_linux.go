//go:build linux

package main

// Pas encore sous Linux (xrandr --rate 50 à brancher ici) : l'écran ne change pas.
func passerFrequence(hz int) (avant, apres int, err error) {
	return 0, 0, nil
}

func remettreFrequence() error { return nil }
