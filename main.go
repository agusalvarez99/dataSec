package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	//"mime"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// obtiene el token, lo guarda y retorna el client http
func getClient(config *oauth2.Config) *http.Client {
	// El archivo token.json almacena los tokens de acceso y de actualización del usuario,
	//  y se crea automáticamente cuando el flujo de autorización se completa por primera vez.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// solicita un token desde la web y luego lo retorna
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Ingresa a este enlace y luego copia el "+
		"codigo de autorizacion: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("No se pudo leer el codigo de autorizacion %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("No se pudo recuperar el token desde la web %v", err)
	}
	return tok
}

// recupera el token a partir de un archivo local
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Guarda el token en un archivo
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Guardando el archivo de credenciales en: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("No se pudo almacenar el token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("No se pudo leer el archivo client secret: %v", err)
	}

	// si modifico el scope tengo que eliminar el token.json viejo
	config, err := google.ConfigFromJSON(b, drive.DriveMetadataScope, gmail.GmailSendScope)
	if err != nil {
		log.Fatalf("No se pudo analizar el archivo client secret: %v", err)
	}
	client := getClient(config)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("No se pudo recuperar el cliente de drive: %v", err)
	}
	//el pagesize me limita la cantidad de archivos para mostrar
	r, err := srv.Files.List().PageSize(2).
		Fields("nextPageToken, files(id, name, fileExtension, owners)").Do()
	if err != nil {
		log.Fatalf("No se pudieron recuperar los archivos: %v", err)
	}
	fmt.Println("App by agusalvarez:")
	if len(r.Files) == 0 {
		fmt.Println("No se encontraron archivos")
	} else {
		fmt.Print("Indique 1 si solo quiere observar los datos o 2 si quiere ademas almacenarlos en la base:")
		var choice string
		_, err := fmt.Scanf("%s", &choice)
		if err != nil {
			log.Fatalf("No se pudo leer la entrada: %v", err)
		}
		for _, i := range r.Files {
			//para determinar si es publico o privado
			permissions, err := srv.Permissions.List(i.Id).Do()
			if err != nil {
				log.Fatalf("No se pudo obtener los permisos del archivo: %v", err)
			}
			visibility := "Privado"
			for _, permiso := range permissions.Permissions {
				if permiso.Type == "anyone" && (permiso.Role == "reader" || permiso.Role == "writer") {
					visibility = "Publico"
					break
				}
			}
			//para determinar cual es la extension del archivo
			fileExtension := i.FileExtension
			file, err := srv.Files.Get(i.Id).Do()
			if err != nil {
				log.Fatalf("No se pudo obtener informacion del archivo: %v", err)
			}

			if strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "folder") {
				fileExtension = "Carpeta de Google"
			} else if strings.Contains(file.MimeType, "google") {
				fileExtension = "Documento de Google"
			}
			//imprimir por pantalla los datos de los archivos obtenidos
			fmt.Printf("\nID: %s\nNombre: %s\nExtensión: %s\nDueño: %s\nVisibilidad: %s\n\n", i.Id, i.Name, fileExtension, i.Owners[0].EmailAddress, visibility)
			if choice == "2" {
				insertFile(i.Id, i.Name, fileExtension, i.Owners[0].EmailAddress, visibility)
			}
		}
	}
	sendEmail()
}

func insertFile(id string, nombre string, extension string, dueño string, visbilidad string) {
	dsn := "root:agustin@tcp(localhost:3306)/inventario"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("No se pudo conectar a la base: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("CALL insert_file(?, ?, ?, ?, ?, ?)", id, nombre, extension, dueño, visbilidad, nil)
	if err != nil {
		log.Fatalf("No se pudo insertar los datos: %v", err)
	}

	fmt.Printf("Datos insertados correctamente!\n")

}

func sendEmail() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("No se pudo leer el archivo client secret: %v", err)
	}

	// configuracion de la autenticacion, en caso de modificar el scope debo borrar el token.json
	config, err := google.ConfigFromJSON(b, gmail.GmailSendScope)
	if err != nil {
		log.Fatalf("No se pudo analizar el archivo client secret: %v", err)
	}
	client := getClient(config)

	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("No se pudo recuperar el cliente de Gmail: %v", err)
	}

	//mensaje a enviar
	to := "agus99test@gmail.com"
	subject := "Cuestionario de seguridad de archivo X"
	body := "enlace al google form"
	message := createMessage(to, subject, body)

	//envio del mensaje
	user := "me"
	_, err = srv.Users.Messages.Send(user, &message).Do()
	if err != nil {
		log.Fatalf("No se pudo enviar el mail: %v", err)
	}
	fmt.Println("Correo enviado correctamente!")
}

func createMessage(to, subject, body string) gmail.Message {
	msg := gmail.Message{}
	headers := make(map[string]string)
	headers["To"] = to
	headers["Subject"] = subject
	headers["Content-Type"] = "text/html; charset\"utf-8\""

	var msgString string
	for k, v := range headers {
		msgString += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msgString += "\r\n" + body

	msg.Raw = base64.URLEncoding.EncodeToString([]byte(msgString))
	return msg
}
