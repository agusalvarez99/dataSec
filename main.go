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
	"google.golang.org/api/sheets/v4"
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
	fmt.Scanln()
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
	config, err := google.ConfigFromJSON(b, drive.DriveMetadataScope, gmail.GmailSendScope, sheets.SpreadsheetsReadonlyScope)
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
	fmt.Println("A continuacion se mostraran los archivos recorridos de a uno por vez!")
	if len(r.Files) == 0 {
		fmt.Println("No se encontraron archivos")
	} else {
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
			//preguntar si desea guardarlo
			fmt.Print("Indique Y si desea guardar los metadatos del archivo en la base de datos de lo contrario ingrese N: ")
			var choice string
			_, err = fmt.Scanln(&choice)
			if err != nil {
				log.Fatalf("No se pudo leer la entrada: %v", err)
			}
			if choice == "y" || choice == "Y" {
				insertFile(i.Id, i.Name, fileExtension, i.Owners[0].EmailAddress, visibility)
			}
			//preguntar si desea enviar por correo las preguntas
			fmt.Println("Desea enviar por correo las preguntas de seguridad? y/N")
			var option string
			// fmt.Scanln()
			_, err = fmt.Scanln(&option)
			if err != nil {
				log.Fatalf("No se pudo leer la entrada: %v", err)
			}
			if option == "y" || option == "Y" {
				sendEmail(i.Id, i.Name, i.Owners[0].EmailAddress)
			}
		}

		fmt.Println("Si ya ha enviado todos los correos y las preguntas han sido contestadas puede procesar las mismas!")
		fmt.Println("Ingrese Y si desea procesar las repuestas o N si desea terminar la ejecucion del programa")
		var choice string
		fmt.Scanln(&choice)
		if choice == "Y" || choice == "y" {
			scanResults()

		} else if choice == "N" || choice == "n" {
			os.Exit(0)
		}
	}
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

func sendEmail(idArchivo, nombreArchivo, dueño string) {
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
	to := dueño
	subject := "Cuestionario de seguridad de archivo " + nombreArchivo + " (" + idArchivo + ")"
	body := "https://forms.gle/bPSC2wxnyCQATwDC9"
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

func scanResults() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("No se pudo leer el client secret: %v", err)
	}

	config, err := google.ConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		log.Fatalf("No se pudo analizar el client secret: %v", err)
	}

	client := getClient(config)

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("No se pudo recuperar el cliente de Sheet: %v", err)
	}

	// ID de la hoja de cálculo y rango
	spreadsheetId := "1XH9MnirbQcUglQo6TwOtEzlDGTI5Hl3RvaUVypgFhvU"
	// rango para recorrer
	readRange := "Respuestas!C2:H"

	resp, err := srv.Spreadsheets.Values.Get(spreadsheetId, readRange).Do()
	if err != nil {
		log.Fatalf("No se pudo obtener los datos de la sheet: %v", err)
	}

	if len(resp.Values) == 0 {
		fmt.Println("No se encontraron datos!")
	} else {
		//ACA TENGO QUE RECORRER LAS FILAS
		for _, row := range resp.Values {
			cellBlank := false
			// el id esta en la columna C
			id := row[0].(string)
			// recorrer los valores de las columnas D a H y calcular puntaje
			var puntaje int
			for i := 1; i <= 5; i++ {
				respuesta := strings.TrimSpace(strings.ToUpper(row[i].(string)))
				if respuesta == "NO" {
					puntaje += 0
				} else if respuesta == "NS/NC" {
					puntaje += 1
				} else if respuesta == "SI" {
					puntaje += 2
				} else if respuesta == "" {
					cellBlank = true
					break
				}
			}
			if cellBlank {
				break
			}

			// asociar nivel de criticidad segun el puntaje
			var nivelCriticidad string
			switch {
			case puntaje >= 0 && puntaje <= 2:
				nivelCriticidad = "BAJO"
			case puntaje >= 3 && puntaje <= 5:
				nivelCriticidad = "MEDIO"
			case puntaje >= 6 && puntaje <= 8:
				nivelCriticidad = "ALTO"
			case puntaje >= 9 && puntaje <= 10:
				nivelCriticidad = "CRITICO"
			default:
				nivelCriticidad = "Desconocido"
			}

			fmt.Printf("ID: %s, Puntaje: %d, Nivel de Criticidad: %s\n", id, puntaje, nivelCriticidad)

			//  conexión con la base de datos MySQL
			db, err := sql.Open("mysql", "root:agustin@tcp(localhost:3306)/inventario")
			if err != nil {
				log.Fatalf("No se pudo conectar a la base: %v", err)
			}
			defer db.Close()

			_, err = db.Exec("CALL add_level(?, ?)", id, nivelCriticidad)
			if err != nil {
				log.Fatalf("No se pudo actualizar el registro: %v", err)
			}
		}

	}

}
