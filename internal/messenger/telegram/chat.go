// Package telegram es el adapter de Telegram: todo lo que sabe de inline
// keyboards, ParseMode HTML, updates y callbacks vive acá y en ningún otro
// lado del árbol.
package telegram

import (
	"context"
	"slices"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/conversation"
)

// buttonsPerRow caps how many inline-keyboard buttons Telegram renders
// per row — putting every option in a single row (the old behavior) is
// what made category/subcategory/account buttons unreadably small.
const buttonsPerRow = 2

// maxLabelForTwoPerRow es el largo a partir del cual una etiqueta ya no entra en
// media pantalla y Telegram la corta.
//
// Con dos por fila, un candidato de corrección ("🔴 Cafe · $2.000 · 27/07")
// llega cortado JUSTO por el final — que es la fecha, o sea lo único que lo
// distingue de los otros dos candidatos. El picker queda inservible: tres
// botones que se leen igual.
//
// El largo es el problema, no la cantidad: las categorías ("🍔 Alimentación")
// entran de a dos y son ~16, así que forzarlas a una por fila duplicaría el
// alto del teclado sin ganar nada.
const maxLabelForTwoPerRow = 20

// htmlParseMode: el resumen semanal usa <b> para que se pueda escanear. Todo
// lo que viene del usuario se escapa en summary.Builder — sin eso Telegram
// devuelve 400 y no llega nada.
const htmlParseMode = models.ParseModeHTML

// chat es una conversación abierta con un chat de Telegram.
type chat struct {
	b      *bot.Bot
	chatID int64
}

func (c chat) Send(ctx context.Context, p conversation.Prompt) error {
	_, err := c.b.SendMessage(ctx, buildParams(c.chatID, p))
	return err
}

func (c chat) Typing(ctx context.Context) error {
	_, err := c.b.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: c.chatID,
		Action: models.ChatActionTyping,
	})
	return err
}

// buildParams traduce un conversation.Prompt neutro al formato real de
// Telegram (botones inline, en grilla de buttonsPerRow por fila).
func buildParams(chatID int64, p conversation.Prompt) *bot.SendMessageParams {
	params := &bot.SendMessageParams{ChatID: chatID, Text: p.Text, ParseMode: htmlParseMode}
	if rows := chunkButtons(p.Buttons); len(rows) > 0 {
		params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	return params
}

// rowWidth decide cuántos botones por fila entran sin que se corte ninguno.
// Alcanza con que UNA etiqueta sea larga: las filas son parejas, así que la más
// larga manda.
func rowWidth(buttons []conversation.Button) int {
	for _, b := range buttons {
		if utf8.RuneCountInString(b.Label) > maxLabelForTwoPerRow {
			return 1
		}
	}
	return buttonsPerRow
}

func chunkButtons(buttons []conversation.Button) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	for chunk := range slices.Chunk(buttons, rowWidth(buttons)) {
		row := make([]models.InlineKeyboardButton, 0, len(chunk))
		for _, btn := range chunk {
			row = append(row, models.InlineKeyboardButton{Text: btn.Label, CallbackData: btn.Data})
		}
		rows = append(rows, row)
	}
	return rows
}
