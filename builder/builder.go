package main

import (
	//"errors"
	"fmt"
	"time"
)

func Build(name string) (string, error) {
	fmt.Println("Building : " + name)
	time.Sleep(0 * time.Second)
	return name + ".deb", nil
}
