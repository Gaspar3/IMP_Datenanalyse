package main

import (
	"fmt"
	"os"
	"strconv"
)

type Todo struct {
	Text string
	Done bool
}

var todos []Todo

func removeIndex(s []Todo, index int) []Todo {
	return append(s[:index], s[index+1:]...)
}

func gasparMain() {
	if len(os.Args) < 3 {
		println("Please type your command")
		return
	}
	command := os.Args[2]

	switch command {

	case "add":
		if len(os.Args) < 3 {
			println("Please add your todo text")
			return
		}
		todos = append(todos, Todo{os.Args[3], false})
		println("succses")

	case "list":
		println("Your current list is: ")
		for i, txt := range todos {
			fmt.Printf("%v: %v", i, txt)
		}

	case "del":
		if len(os.Args) < 3 {
			println("Please add an index to delete")
			return
		}

		i, err := strconv.Atoi(os.Args[3])
		if err != nil {
			println("Please add an correct index")
			return
		}

		todos = removeIndex(todos, i)
		fmt.Printf("Your todo list is now: %v", todos)

	default:
		println("Command not found")
	}
}
