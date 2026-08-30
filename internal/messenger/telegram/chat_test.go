package telegram

import (
	"testing"

	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/conversation"
)

// La grilla es 2 por fila salvo que UNA etiqueta sea larga: las filas son
// parejas, así que la más larga manda. Ver el comentario de
// maxLabelForTwoPerRow — el picker de correcciones queda inservible si se corta.
func TestChunkButtons_TwoPerRow(t *testing.T) {
	btns := []conversation.Button{
		{Label: "🍔 Alimentación", Data: "a"},
		{Label: "🚗 Transporte", Data: "b"},
		{Label: "🏠 Hogar", Data: "c"},
	}
	rows := chunkButtons(btns, "")
	if len(rows) != 2 {
		t.Fatalf("filas = %d, want 2", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 1 {
		t.Errorf("anchos = %d,%d; want 2,1", len(rows[0]), len(rows[1]))
	}
	if rows[0][0].Text != "🍔 Alimentación" || rows[0][0].CallbackData != "a" {
		t.Errorf("botón mal traducido: %+v", rows[0][0])
	}
}

func TestChunkButtons_OnePerRowWhenALabelIsLong(t *testing.T) {
	btns := []conversation.Button{
		{Label: "🔴 Cafe · $2.000 · 27/07", Data: "a"}, // 24 runas > 20
		{Label: "corto", Data: "b"},
	}
	rows := chunkButtons(btns, "")
	if len(rows) != 2 {
		t.Fatalf("filas = %d, want 2 (una por fila)", len(rows))
	}
	for i, r := range rows {
		if len(r) != 1 {
			t.Errorf("fila %d tiene %d botones, want 1", i, len(r))
		}
	}
}

func TestChunkButtons_EmptyIsNil(t *testing.T) {
	if rows := chunkButtons(nil, ""); rows != nil {
		t.Errorf("sin botones no hay teclado, hubo %d filas", len(rows))
	}
}

// El ParseMode es un telegramismo y vive acá, no en el core.
func TestBuildParams_AlwaysHTMLAndKeyboardOnlyWhenThereAreButtons(t *testing.T) {
	p := buildParams(42, conversation.Prompt{Text: "hola"}, "")
	if p.ParseMode != htmlParseMode {
		t.Errorf("ParseMode = %q, want %q", p.ParseMode, htmlParseMode)
	}
	if p.ReplyMarkup != nil {
		t.Error("un texto sin botones no lleva ReplyMarkup")
	}
	if p.ChatID != int64(42) {
		t.Errorf("ChatID = %v, want 42", p.ChatID)
	}

	withBtns := buildParams(42, conversation.Prompt{
		Text:    "elegí",
		Buttons: []conversation.Button{{Label: "sí", Data: "yes"}},
	}, "")
	if withBtns.ReplyMarkup == nil {
		t.Error("con botones tiene que haber ReplyMarkup")
	}
}

func TestBuildParams_WebAppButtonUsesBaseHost(t *testing.T) {
	p := conversation.Prompt{
		Text:    "hola",
		Buttons: []conversation.Button{{Label: "Ver julio", WebAppPath: "/app/overview?p=month&m=2026-07"}},
	}

	params := buildParams(7, p, "https://lopii.example")

	markup, ok := params.ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("ReplyMarkup = %T, want *models.InlineKeyboardMarkup", params.ReplyMarkup)
	}
	btn := markup.InlineKeyboard[0][0]
	if btn.WebApp == nil {
		t.Fatal("WebApp is nil, want the deep link")
	}
	if btn.WebApp.URL != "https://lopii.example/app/overview?p=month&m=2026-07" {
		t.Errorf("URL = %q", btn.WebApp.URL)
	}
	if btn.CallbackData != "" {
		t.Errorf("CallbackData = %q, want empty — Telegram rejects both on one button", btn.CallbackData)
	}
}

func TestBuildParams_CallbackButtonUnchanged(t *testing.T) {
	p := conversation.Prompt{
		Text:    "hola",
		Buttons: []conversation.Button{{Label: "Sí", Data: "yes"}},
	}

	params := buildParams(7, p, "https://lopii.example")

	markup := params.ReplyMarkup.(*models.InlineKeyboardMarkup)
	btn := markup.InlineKeyboard[0][0]
	if btn.CallbackData != "yes" {
		t.Errorf("CallbackData = %q, want %q", btn.CallbackData, "yes")
	}
	if btn.WebApp != nil {
		t.Errorf("WebApp = %+v, want nil", btn.WebApp)
	}
}
