package celestia

// DerivationVersionCelestia is a byte marker for celestia references submitted
// to the batch inbox address as calldata.
// Mnemonic 0xce = celestia
// version 0xce references are encoded as:
// [8]byte block height ++ [32]byte commitment
// in little-endian encoding.
// see: https://github.com/rollkit/celestia-da/blob/1f2df375fd2fcc59e425a50f7eb950daa5382ef0/celestia.go#L141-L160
const DerivationVersionCelestia = 0xce
