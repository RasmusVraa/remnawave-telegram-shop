//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestSupportAttachments_insertListAndOwnership(t *testing.T) {
	pool := pgPoolIntegration(t)
	ctx := context.Background()
	accounts := NewAccountRepo(pool)
	support := NewSupportRepo(pool)

	owner, err := accounts.Create(ctx, "att-owner-"+uuid.NewString()+"@example.com", "x", "ru")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := accounts.Create(ctx, "att-stranger-"+uuid.NewString()+"@example.com", "x", "ru")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM cabinet_account WHERE id = ANY($1)`, []int64{owner.ID, stranger.ID})
	})

	w, h, size := 1280, 720, 4096
	name := "IMG_1.heic"
	var msgID int64
	var saved []SupportAttachment
	err = support.WithTx(ctx, func(tx pgx.Tx) error {
		ticket, err := support.CreateTicketTx(ctx, tx, owner.ID)
		if err != nil {
			return err
		}
		// Сообщение из одних картинок — с пустым текстом.
		msg, err := support.InsertMessageTx(ctx, tx, &SupportMessage{
			TicketID: ticket.ID, Direction: SupportMsgIn, Text: "", DeliveryStatus: SupportDeliverySent,
		})
		if err != nil {
			return err
		}
		msgID = msg.ID
		saved, err = support.InsertAttachmentsTx(ctx, tx, msg.ID, []SupportAttachment{
			{SupportBotAttachmentID: 9001, Mime: "image/jpeg", Width: &w, Height: &h, SizeBytes: &size},
			{SupportBotAttachmentID: 9002, Mime: "image/heic", Name: &name},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 || saved[0].Position != 0 || saved[1].Position != 1 || saved[1].Width != nil {
		t.Fatalf("saved: %+v", saved)
	}

	byMsg, err := support.ListAttachmentsByMessages(ctx, []int64{msgID, -1})
	if err != nil {
		t.Fatal(err)
	}
	got := byMsg[msgID]
	if len(got) != 2 || got[0].SupportBotAttachmentID != 9001 || *got[0].Width != 1280 || *got[1].Name != name {
		t.Fatalf("listed: %+v", got)
	}
	if empty, err := support.ListAttachmentsByMessages(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("no ids: %v %v", empty, err)
	}

	a, err := support.GetAttachmentForAccount(ctx, saved[1].ID, owner.ID)
	if err != nil || a.Mime != "image/heic" {
		t.Fatalf("owner: %+v %v", a, err)
	}
	if _, err := support.GetAttachmentForAccount(ctx, saved[1].ID, stranger.ID); !errors.Is(err, ErrSupportNotFound) {
		t.Fatalf("stranger must not see the attachment: %v", err)
	}
	if _, err := support.GetAttachmentForAccount(ctx, -1, owner.ID); !errors.Is(err, ErrSupportNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
