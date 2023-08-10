// main_test.go
package main

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsertFile(t *testing.T) {
	insertFile("abc123", "nombreFile", "extFile", "dueñoFile", "visibFile")
}

func TestConectionDB(t *testing.T) {
	db, err := connectionDb()
	if err != nil {
		t.Fatalf("No se pudo conectar a la base: %v", err)
	}
	defer db.Close()

	// ejecutar una query simple para verificar que la conexión funcione
	_, err = db.Query("SELECT 1")
	if err != nil {
		t.Fatalf("Error al ejecutar la consulta: %v", err)
	}
}

func TestCreateMessage(t *testing.T) {
	to := "agus@ejemplo.com"
	subject := "AsuntoTest"
	body := "CuerpoTest"

	msg := createMessage(to, subject, body)

	// Decodifica el mensaje y verifica que coincidan destinatario, asunto y body con lo esperado
	decodedMessage, err := base64.URLEncoding.DecodeString(msg.Raw)
	assert.NoError(t, err)
	assert.Contains(t, string(decodedMessage), body)
	assert.Contains(t, string(decodedMessage), to)
	assert.Contains(t, string(decodedMessage), subject)

}
