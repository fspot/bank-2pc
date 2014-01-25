package mylib

type MoneyRequest struct {
	Cid int
}

type MoneyResponse struct {
	Ok    bool
	Money int
}

type TransferRequest struct {
	Cid        int
	FriendIBAN int
	Amount     int
}

type TransferResponse struct {
	Ok bool
}

type CoordRequest struct {
	Src    int
	Dest   int
	Amount int
}

type CoordResponse struct {
	Ok bool
}

type SudoRequest struct {
	Cid           int
	Amount        int
	TransactionID int64
}

type SudoResponse struct {
	Vote bool
}

type OrderRequest struct {
	Commit        bool
	TransactionID int64
}

type OrderResponse struct {
	Ok bool
}
