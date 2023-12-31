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
	"github.com/joho/godotenv"
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
	fmt.Println("App by agusalvarez:")
	fmt.Println("Ingrese\n A si desea pasar a analizar los archivos de su unidad\n Anykey si desea pasar a las siguientes opciones!")
	var decision string
	fmt.Scanln(&decision)
	if decision == "A" || decision == "a" {
		analyzeFiles()
	} else {
		fmt.Println("Si ya ha enviado todos los correos y las preguntas han sido contestadas puede procesar las mismas!")
		fmt.Println("Ingrese\n Y si desea procesar las repuestas\n N si quiere ejecutar el Leak Prevention\n Anykey si desea terminar la ejecucion del programa: ")
		var choice string
		fmt.Scanln(&choice)
		if choice == "Y" || choice == "y" {
			scanResults()
		} else if choice == "N" || choice == "n" {
			leakagePrevention()
		} else {
			fmt.Println("Programa finalizado! ")
			os.Exit(0)
		}
	}
}

func insertFile(id string, nombre string, extension string, dueño string, visbilidad string) {
	db, err := connectionDb()
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
	config, err := google.ConfigFromJSON(b, drive.DriveScope, gmail.GmailSendScope, sheets.SpreadsheetsReadonlyScope)
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
	subject := "Cuestionario de seguridad de archivo " + nombreArchivo
	body := "El ID de su archivo es " + idArchivo + " y el enlace a su cuestionario es: https://forms.gle/bPSC2wxnyCQATwDC9"
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
	//creamos el mail pero vacio
	msg := gmail.Message{}
	//hacemos un map para poner poner los headers
	headers := make(map[string]string)
	headers["To"] = to
	headers["Subject"] = subject
	headers["Content-Type"] = "text/html; charset\"utf-8\""

	//armo un string para esos headers y luego despues de estos agrego el body
	var msgString string
	for k, v := range headers {
		msgString += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msgString += "\r\n" + body
	//codificamos en base64 para poder agregarlo al Raw del correo
	msg.Raw = base64.URLEncoding.EncodeToString([]byte(msgString))
	return msg
}

func scanResults() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("No se pudo leer el client secret: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope, gmail.GmailSendScope, sheets.SpreadsheetsReadonlyScope)
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
	// rango para recorrer (solo respuestas) no conozco el final
	readRange := "Respuestas!C2:H"

	resp, err := srv.Spreadsheets.Values.Get(spreadsheetId, readRange).Do()
	if err != nil {
		log.Fatalf("No se pudo obtener los datos de la sheet: %v", err)
	}

	if len(resp.Values) == 0 {
		fmt.Println("No se encontraron datos!")
	} else {
		//ACA TENGO QUE RECORRER LAS FILAS, deja de recorrer cuando encuentra una en blanco
		for _, row := range resp.Values {
			cellBlank := false
			// el id esta en la columna C (que es la primera dentro del rango que yo defini)
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
			//muestro por pantalla para saber que anda todo ok
			fmt.Printf("ID: %s, Puntaje: %d, Nivel de Criticidad: %s\n", id, puntaje, nivelCriticidad)

			//  conexión con la base de datos
			db, err := connectionDb()
			if err != nil {
				log.Fatalf("No se pudo conectar a la base: %v", err)
			}
			defer db.Close()
			// actualizamos la criticidad del archivo
			_, err = db.Exec("CALL add_level(?, ?)", id, nivelCriticidad)
			if err != nil {
				log.Fatalf("No se pudo actualizar el registro: %v", err)
			}
		}

	}

}

func leakagePrevention() {
	//  conexión con la base de datos
	db, err := connectionDb()
	if err != nil {
		log.Fatalf("No se pudo conectar a la base: %v", err)
	}
	defer db.Close()
	//obtengo los registros que debo modificar segun reglas del challenge
	rows, err := db.Query("CALL getCriticos()")
	if err != nil {
		log.Fatalf("No se pudo obtener los registros: %v", err)
	}
	defer rows.Close()

	// recorro las filas resultado de la query anterior que solo retorna los IDs correspondientes
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			log.Fatalf("No se pudo obtener el id: %v", err)
		}

		// aca hacer la actualizacion de permisos del archivo con id que esta siendo iterado
		updatePerm(id)

		//aca hacer el update en la base de datos de ese id y pasarlo de Publico a Privado
		_, err = db.Exec("CALL updateVisibility(?)", id)
		if err != nil {
			log.Fatalf("No se pudo actualizar el campo: %v", err)
		}

	}
	if err := rows.Err(); err != nil {
		log.Fatalf("Ocurrio un error durante las iteraciones en los resultados de la query: %v", err)
	}

}

func updatePerm(id string) {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("No se pudo leer el archivo client secret: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope, gmail.GmailSendScope, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		log.Fatalf("No se pudo analizar el secret client: %v", err)
	}

	client := getClient(config)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("No se pudo obtener el cliente de Drive: %v", err)
	}
	//este es el ID correspondiente a los permisos publicos, hay que eliminarlo no modificarlo
	//los demas permisos tienen un ID asociado al usuario de ese permiso especifico
	idPermiso := "anyoneWithLink"

	//delete ese permiso y de esa forma deja de ser publico
	err = srv.Permissions.Delete(id, idPermiso).Do()
	if err != nil {
		log.Fatalf("No se pudo eliminar el permiso: %v", err)
	}

}

func analyzeFiles() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("No se pudo leer el archivo client secret: %v", err)
	}

	// si modifico el scope tengo que eliminar el token.json viejo
	config, err := google.ConfigFromJSON(b, drive.DriveScope, gmail.GmailSendScope, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		log.Fatalf("No se pudo analizar el archivo client secret: %v", err)
	}
	client := getClient(config)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("No se pudo recuperar el cliente de drive: %v", err)
	}

	//para que no me muestre las carpetas
	q := "mimeType != 'application/vnd.google-apps.folder'"

	var cantidad int64
	fmt.Println("Indique la cantidad de archivos que desea iterar: ")
	_, err = fmt.Scanln(&cantidad)
	if err != nil {
		log.Fatalf("No se pudo leer la entrada: %v", err)
	}

	//el pagesize me limita la cantidad de archivos que voy a recorrer en la unidad de drive
	//si pongo de mas no hay problema, corta despues del ultimo
	r, err := srv.Files.List().PageSize(cantidad).Q(q).
		Fields("nextPageToken, files(id, name, fileExtension, owners)").Do()
	if err != nil {
		log.Fatalf("No se pudieron recuperar los archivos: %v", err)
	}

	fmt.Println("A continuacion se mostraran los archivos recorridos de a uno por vez!")
	if len(r.Files) == 0 {
		fmt.Println("No se encontraron archivos")
	} else {
		//pasamos a iterar sobre los archivos
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
			fileExtension := findExtension(srv, i)

			//imprimir por pantalla los datos de los archivos obtenidos
			fmt.Printf("\nID: %s\nNombre: %s\nExtensión: %s\nDueño: %s\nVisibilidad: %s\n\n", i.Id, i.Name, fileExtension, i.Owners[0].EmailAddress, visibility)
			//preguntar si desea guardarlo
			fmt.Println("Indique Y si desea guardar los metadatos del archivo en la base de datos de lo contrario ingrese N: ")
			var choice string
			_, err = fmt.Scanln(&choice)
			if err != nil {
				log.Fatalf("No se pudo leer la entrada: %v", err)
			}
			if choice == "y" || choice == "Y" {
				insertFile(i.Id, i.Name, fileExtension, i.Owners[0].EmailAddress, visibility)
			}
			//preguntar si desea enviar por correo las preguntas
			fmt.Println("Desea enviar por correo las preguntas de seguridad?\n Ingrese Y para si\n Ingrese N para no: ")
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
	}
}

func connectionDb() (*sql.DB, error) {
	//  cargamos las var de entorno
	err := godotenv.Load("dbCred.env")
	if err != nil {
		log.Fatalf("No se pudo cargar el archivo: %v", err)
	}
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	//string de conexion
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("No se pudo conectar a la base: %v", err)
	}
	//retorno el objeto sql.DB
	return db, err
}

func findExtension(srv *drive.Service, i *drive.File) string {
	var fileExtension string
	file, err := srv.Files.Get(i.Id).Do()
	if err != nil {
		log.Fatalf("No se pudo obtener informacion del archivo: %v", err)
	}
	switch {
	case strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "document"):
		fileExtension = "Documento de Google"
	case strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "form"):
		fileExtension = "Form"
	case strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "jam"):
		fileExtension = "Jam"
	case strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "photo"):
		fileExtension = "Photo"
	case strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "script"):
		fileExtension = "Script"
	case strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "site"):
		fileExtension = "Site"
	case strings.Contains(file.MimeType, "google") && strings.Contains(file.MimeType, "spreadsheet"):
		fileExtension = "Spreadsheet"
	default:
		fileExtension = i.FileExtension
	}
	return fileExtension
}
