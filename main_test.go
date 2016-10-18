package main

import (
	"testing"
	"github.com/kataras/iris"
	"github.com/kataras/iris/httptest"
)

func TestMain(t *testing.T) {
	setApi()
}

func TestLogin(t *testing.T) {
	httptest.New(iris.Default, t).GET(iris.Path("login")).Expect().Body().Match("login") 	
}

func TestLoginAccept(t *testing.T) {
	// post without service
	//httptest.New(iris.Default, t).POST(iris.Path("login")).Expect().Body().Match("login.+ST") 	
	httptest.New(iris.Default, t).POST(iris.Path("login")).WithFormField("username", "john").WithFormField("password", "john").Expect().Body().Match("successful")
}

func TestLogoutNoUser(t *testing.T) {
	httptest.New(iris.Default, t).GET(iris.Path("logout")).Expect().Body().Match("User is not logged in") 	
}

func TestLogoutUserCorrect(t *testing.T) {
	httptest.New(iris.Default, t).GET(iris.Path("logout")).WithCookie("TGCGOLACAS", "lllll").Expect().Body().Match("User is not logged in") 	
}


func TestValidateNoTicketNoService(t *testing.T) {
	httptest.New(iris.Default, t).GET(iris.Path("validate")).Expect().Body().Match("no") 	
}

func TestValidateExistingTicket(t *testing.T) {
	ticket := NewTicket("ST", "", "jmcarbo", false)
	httptest.New(iris.Default, t).GET(iris.Path("validate")).WithQuery("ticket", ticket.Value).Expect().Body().Match("yes\njmcarbo") 	
}
