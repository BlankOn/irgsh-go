package main

import (
	//"errors"
	"fmt"
	"time"
)

func Repo(name string) (string, error) {
	fmt.Println("Rebuilding repo : " + name)
	time.Sleep(0 * time.Second)
	return "/blankon/pool/main/i/", nil
}
