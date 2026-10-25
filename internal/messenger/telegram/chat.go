package telegram

import (
	"context"
	"slices"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/conversation"
)

const buttonsPerRow = 2

const maxLabelForTwoPerRow = 20

const htmlParseMode = models.ParseModeHTML

type chat struct {
	b               *bot.Bot
	chatID          int64
	baseHost        string
	sourceMessageID int
}

func (c chat) Send(ctx context.Context, p conversation.Prompt) error {
	_, err := c.b.SendMessage(ctx, buildParams(c.chatID, p, c.baseHost))
	return err
}

func (c chat) EditSource(ctx context.Context, p conversation.Prompt) error {
	if c.sourceMessageID == 0 {
		return c.Send(ctx, p)
	}
	_, err := c.b.EditMessageText(ctx, buildEditParams(c.chatID, c.sourceMessageID, p, c.baseHost))
	return err
}

func (c chat) ClearSourceButtons(ctx context.Context) error {
	if c.sourceMessageID == 0 {
		return nil
	}
	_, err := c.b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      c.chatID,
		MessageID:   c.sourceMessageID,
		ReplyMarkup: emptyKeyboard(),
	})
	return err
}

func buildEditParams(chatID int64, messageID int, p conversation.Prompt, baseHost string) *bot.EditMessageTextParams {
	markup := emptyKeyboard()
	if rows := chunkButtons(p.Buttons, baseHost); len(rows) > 0 {
		markup.InlineKeyboard = rows
	}
	return &bot.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: p.Text, ParseMode: htmlParseMode, ReplyMarkup: markup}
}

func emptyKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
}

func (c chat) Typing(ctx context.Context) error {
	_, err := c.b.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: c.chatID,
		Action: models.ChatActionTyping,
	})
	return err
}

func buildParams(chatID int64, p conversation.Prompt, baseHost string) *bot.SendMessageParams {
	params := &bot.SendMessageParams{ChatID: chatID, Text: p.Text, ParseMode: htmlParseMode}
	if rows := chunkButtons(p.Buttons, baseHost); len(rows) > 0 {
		params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	return params
}

func rowWidth(buttons []conversation.Button) int {
	for _, b := range buttons {
		if utf8.RuneCountInString(b.Label) > maxLabelForTwoPerRow {
			return 1
		}
	}
	return buttonsPerRow
}

func chunkButtons(buttons []conversation.Button, baseHost string) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	for chunk := range slices.Chunk(buttons, rowWidth(buttons)) {
		row := make([]models.InlineKeyboardButton, 0, len(chunk))
		for _, btn := range chunk {
			if btn.WebAppPath != "" {
				row = append(row, models.InlineKeyboardButton{
					Text:   btn.Label,
					WebApp: &models.WebAppInfo{URL: baseHost + btn.WebAppPath},
				})
				continue
			}
			row = append(row, models.InlineKeyboardButton{Text: btn.Label, CallbackData: btn.Data})
		}
		rows = append(rows, row)
	}
	return rows
}
