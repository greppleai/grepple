package testdata

import "fmt"

type User struct {
	ID   string
	Name string
}

func FormatUser(user User) string {
	return fmt.Sprintf("%s (%s)", user.Name, user.ID)
}

type Reporter struct{}

func (Reporter) PrintUser(user User) {
	fmt.Println(FormatUser(user))
}
