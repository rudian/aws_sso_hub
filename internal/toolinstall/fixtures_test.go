package toolinstall

// Recorded `pkgutil --check-signature` outputs (the names are made up; the lines are the shapes recorded on
// 2026-11-16 from Homebrew 7.0.13 and from the damaged copies of the S0-04 test).
const sigGood = `Package "Homebrew.pkg":
   Status: signed by a developer certificate issued by Apple for distribution
   Notarization: trusted by the Apple notary service
   Signed with a trusted timestamp on: 2026-11-09 08:30:26 +0000
   Certificate Chain:
    1. Developer ID Installer: Example Maintainer (927JGANW46)
       Expires: 2029-07-02 07:54:16 +0000
       SHA256 Fingerprint:
           3C 5D AA 66 55 A8 89 1F C1 E0 C0 60 8A EB 6B 3D 06 54 9E 05 A6 7C 1D C9 03 CE A8 D5 21 20 AA C0
       ------------------------------------------------------------------------
    2. Developer ID Certification Authority
       Expires: 2031-09-17 00:00:00 +0000
       ------------------------------------------------------------------------
    3. Apple Root CA
       Expires: 2035-02-09 21:40:36 +0000
`

const sigWrongTeam = `Package "Homebrew.pkg":
   Status: signed by a developer certificate issued by Apple for distribution
   Notarization: trusted by the Apple notary service
   Certificate Chain:
    1. Developer ID Installer: Another Maintainer (ABCDEFGHIJ)
    2. Developer ID Certification Authority
    3. Apple Root CA
`

const sigNotNotarized = `Package "Homebrew.pkg":
   Status: signed by a developer certificate issued by Apple for distribution
   Certificate Chain:
    1. Developer ID Installer: Example Maintainer (927JGANW46)
    2. Developer ID Certification Authority
    3. Apple Root CA
`

const sigInvalid = `Package "Homebrew.pkg":
   Status: package is invalid (checksum did not verify)
`

const sigUnsigned = `Package "Homebrew.pkg":
   Status: no signature
`

const sigNotAPackage = `Could not open package at path: Homebrew.pkg
`

// A certificate that is not a Developer ID Installer (an Apple Development or a Mac Installer leaf).
const sigOtherLeaf = `Package "Homebrew.pkg":
   Status: signed by a developer certificate issued by Apple for distribution
   Notarization: trusted by the Apple notary service
   Certificate Chain:
    1. Mac Developer Installer: Example Maintainer (927JGANW46)
    2. Apple Worldwide Developer Relations Certification Authority
    3. Apple Root CA
`

var sigFixtures = map[string]string{
	"good": sigGood, "wrongteam": sigWrongTeam, "notnotarized": sigNotNotarized, "invalid": sigInvalid,
	"unsigned": sigUnsigned, "notapkg": sigNotAPackage, "otherleaf": sigOtherLeaf,
}
