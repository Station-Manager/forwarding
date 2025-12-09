package qrz

import (
	"github.com/Station-Manager/errors"
	"github.com/Station-Manager/types"
)

// updateDatabase updates the local database with the provided QSO record.
// Returns an error if the database update process fails.
// We update the database here rather than at the caller because different services have different requirements
// concerning the fields and values that are stored in the database.
func (s *Service) updateDatabase(qso types.Qso) error {
	const op errors.Op = "forwarder.qrz.updateDatabase"
	if qso.ID < 1 {
		return errors.New(op).Msgf("invalid QSO ID, unable to update: %d", qso.ID)
	}

	//model.QrzcomQsoUploadStatus = null.StringFrom("Y")
	//model.QrzcomQsoUploadDate = null.StringFrom(utils.DateNowAsYYYYMMDD())
	//
	//if _, err = model.Update(context.Background(), s.DatabaseService.Conn(), boil.Infer()); err != nil {
	//	return errors.New(op).WithErrorf("updating database: %w", err)
	//}

	return nil
}
