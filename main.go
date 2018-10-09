// main.go

package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/kataras/iris"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmail "google.golang.org/api/gmail/v1"
)

const uploadsDir = "./public/uploads/"

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	defer f.Close()
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	json.NewEncoder(f).Encode(token)
}

func createMessage(from string, to string, subject string, content string) gmail.Message {

	var message gmail.Message

	messageBody := []byte("From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n\r\n" +
		content)

	// see https://godoc.org/google.golang.org/api/gmail/v1#Message on .Raw
	message.Raw = base64.StdEncoding.EncodeToString(messageBody)

	return message
}

func chunkSplit(body string, limit int, end string) string {

	var charSlice []rune

	// push characters to slice
	for _, char := range body {
		charSlice = append(charSlice, char)
	}

	var result string = ""

	for len(charSlice) >= 1 {
		// convert slice/array back to string
		// but insert end at specified limit

		result = result + string(charSlice[:limit]) + end

		// discard the elements that were copied over to result
		charSlice = charSlice[limit:]

		// change the limit
		// to cater for the last few words in
		//
		if len(charSlice) < limit {
			limit = len(charSlice)
		}

	}

	return result

}

func randStr(strSize int, randType string) string {

	var dictionary string

	if randType == "alphanum" {
		dictionary = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	}

	if randType == "alpha" {
		dictionary = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	}

	if randType == "number" {
		dictionary = "0123456789"
	}

	var bytes = make([]byte, strSize)
	rand.Read(bytes)
	for k, v := range bytes {
		bytes[k] = dictionary[v%byte(len(dictionary))]
	}
	return string(bytes)
}

func createMessageWithAttachment(from string, to string, subject string, content string, fileDir string, fileName string) gmail.Message {

	var message gmail.Message

	// read file for attachment purpose
	// ported from https://developers.google.com/gmail/api/sendEmail.py

	fileBytes, err := ioutil.ReadFile(fileDir + fileName)
	if err != nil {
		log.Fatalf("Unable to read file for attachment: %v", err)
	}

	fileMIMEType := http.DetectContentType(fileBytes)

	// https://www.socketloop.com/tutorials/golang-encode-image-to-base64-example
	fileData := base64.StdEncoding.EncodeToString(fileBytes)

	boundary := randStr(32, "alphanum")

	messageBody := []byte("Content-Type: multipart/mixed; boundary=" + boundary + " \n" +
		"MIME-Version: 1.0\n" +
		"to: " + to + "\n" +
		"from: " + from + "\n" +
		"subject: " + subject + "\n\n" +

		"--" + boundary + "\n" +
		"Content-Type: text/plain; charset=" + string('"') + "UTF-8" + string('"') + "\n" +
		"MIME-Version: 1.0\n" +
		"Content-Transfer-Encoding: 7bit\n\n" +
		content + "\n\n" +
		"--" + boundary + "\n" +

		"Content-Type: " + fileMIMEType + "; name=" + string('"') + fileName + string('"') + " \n" +
		"MIME-Version: 1.0\n" +
		"Content-Transfer-Encoding: base64\n" +
		"Content-Disposition: attachment; filename=" + string('"') + fileName + string('"') + " \n\n" +
		chunkSplit(fileData, 76, "\n") +
		"--" + boundary + "--")

	// see https://godoc.org/google.golang.org/api/gmail/v1#Message on .Raw
	// use URLEncoding here !! StdEncoding will be rejected by Google API

	message.Raw = base64.URLEncoding.EncodeToString(messageBody)

	return message
}

func main() {

	//*************************************************************************
	//				READING CREDENTIALS
	//*************************************************************************
	b, err := ioutil.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	//*************************************************************************
	//		CONFIG
	// If modifying these scopes, delete your previously saved client_secret.json.
	//*************************************************************************
	config, err := google.ConfigFromJSON(b, gmail.GmailSendScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	//*************************************************************************
	// 			INITIALIZING SERVICE
	//*************************************************************************
	srv, err := gmail.New(getClient(config))
	if err != nil {
		log.Fatalf("Unable to retrieve Gmail client: %v", err)
	}

	//*************************************************************************
	//
	//		START SENDING WITHOUT ATTACHMENT
	//
	//*************************************************************************
	// msg := createMessage("albertputrapurnama@gmail.com", "albertputrapurnama@gmail.com", "Hi! ", "Trial!")
	// user := "me"
	// _, err = srv.Users.Messages.Send(user, &msg).Do()
	// if err != nil {
	// 	log.Fatalf("Unable to send message: %v", err)
	// }
	//*************************************************************************
	//
	//		END SENDING WITHOUT ATTACHMENT
	//
	//*************************************************************************

	//*************************************************************************
	//
	//		START SENDING WITH ATTACHMENT
	//
	//*************************************************************************

	// messageWithAttachment := createMessageWithAttachment("albertputrapurnama@gmail.com", "albertputrapurnama@gmail.com", "Email WITH ATTACHMENT from GMail API", "Trial", "./public/uploads/", "stofication.png")
	// user := "me"
	// _, err = srv.Users.Messages.Send(user, &messageWithAttachment).Do()
	// if err != nil {
	// 	log.Fatalf("Unable to send message: %v", err)
	// }

	//*************************************************************************
	//
	//		END SENDING WITH ATTACHMENT
	//
	//*************************************************************************

	app := iris.New()

	// Register templates
	app.RegisterView(iris.HTML("./views", ".html"))

	// Make the /public route path to statically serve the ./public/... contents
	app.StaticWeb("/public", "./public")

	// Render the actual form
	// GET: http://localhost:8080
	app.Get("/", func(ctx iris.Context) {
		ctx.View("upload.html")
	})

	// Upload the file to the server
	// POST: http://localhost:8080/upload
	app.Post("/upload", iris.LimitRequestBodySize(10<<20), func(ctx iris.Context) {
		// Get the file from the dropzone request
		file, info, err := ctx.FormFile("file")
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Application().Logger().Warnf("Error while uploading: %v", err.Error())
			return
		}

		defer file.Close()
		fname := info.Filename

		// Create a file with the same name
		// assuming that you have a folder named 'uploads'
		out, err := os.OpenFile(uploadsDir+fname,
			os.O_WRONLY|os.O_CREATE, 0666)

		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Application().Logger().Warnf("Error while preparing the new file: %v", err.Error())
			return
		}
		defer out.Close()

		//**********************************************************************
		//				SENDING EMAIL
		//			USING THE GMAIL API
		//**********************************************************************
		msgContent := "You just uploaded this file!"
		msgSubject := "Sent from dropzone-go"
		msgWithAttachment := createMessageWithAttachment("dropzone-go@gmail.com", "albertputrapurnama@gmail.com", msgSubject, msgContent, "./public/uploads/", fname)
		userID := "me"
		_, err = srv.Users.Messages.Send(userID, &msgWithAttachment).Do()
		if err != nil {
			log.Fatal("Unable to send email error: %v", err)
		}

		io.Copy(out, file)
	})

	// Start the server at http://localhost:8080
	app.Run(iris.Addr(":8080"))
}
