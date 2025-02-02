package main

import (
	"fmt"
	"math/rand"
	"os"
)

func SaveData1(path string, data []byte) error {
	// this opens the file in write only mode
	// if it doesn't exist it will create it
	// it it doest exist it will truncate it
	fp, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0664)
	if err != nil {
		return err
	}

	defer fp.Close()

	// keeps all the data in memory
	_, err = fp.Write(data)
	if err != nil {
		return err
	}
	return fp.Sync()
}

func SaveData2(path string, data []byte) error {
	tmp := fmt.Sprintf("%s.tmp.%d", path, rand.Int())
	// os.O_EXCL makes the open file fail if the file exists
	fp, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 06664)
	if err != nil {
		return err
	}
	defer func() {
		fp.Close()
		if err != nil {
			os.Remove(tmp)
		}
	}()

	_, err = fp.Write(data)
	if err != nil {
		return err
	}
	err = fp.Sync() //fsync
	if err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
