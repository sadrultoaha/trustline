#!/usr/bin/env python3
"""Generate testdata/vendors/northwind-soc2-type2.pdf: a fictional SOC 2 Type II
report used by tests and the demo. Pure stdlib: writes a text-only PDF
(Helvetica, uncompressed content streams) so any PDF reader can extract it."""
import sys

PAGES = [
  # (section heading shown in the header, list of lines)
  ("Cover", [
    ("H1", "Northwind Cloud Storage, Inc."),
    ("H2", "SOC 2 Type II Report"),
    ("P", "Report on Northwind's Description of Its Object Storage Platform and on the"),
    ("P", "Suitability of the Design and Operating Effectiveness of Its Controls Relevant"),
    ("P", "to Security, Availability and Confidentiality"),
    ("P", ""),
    ("P", "For the period January 1, 2026 to June 30, 2026"),
    ("P", ""),
    ("P", "Prepared by: Halvorsen & Ruiz LLP, Certified Public Accountants"),
    ("P", "Report date: August 14, 2026"),
    ("P", "CONFIDENTIAL - for use by Northwind customers and their auditors only"),
  ]),
  ("Table of Contents", [
    ("H2", "Table of Contents"),
    ("P", "Section 1  Independent Service Auditor's Report ............ 3"),
    ("P", "Section 2  Management's Assertion ......................... 4"),
    ("P", "Section 3  Description of the System ...................... 5"),
    ("P", "Section 4  Trust Services Criteria, Controls and Results ... 6"),
    ("P", "Section 5  Other Information Provided by Management ........ 9"),
  ]),
  ("Section 1 Independent Service Auditor's Report", [
    ("H2", "Section 1 - Independent Service Auditor's Report"),
    ("P", "To the Management of Northwind Cloud Storage, Inc.:"),
    ("P", "We have examined Northwind's description of its object storage platform for the"),
    ("P", "period January 1, 2026 to June 30, 2026 and the suitability of the design and"),
    ("P", "operating effectiveness of controls to provide reasonable assurance that the"),
    ("P", "service commitments and system requirements were achieved based on the trust"),
    ("P", "services criteria relevant to security, availability and confidentiality."),
    ("H3", "Opinion"),
    ("P", "In our opinion, in all material respects, the description presents the system"),
    ("P", "that was designed and implemented throughout the period, and the controls stated"),
    ("P", "in the description were suitably designed and operated effectively throughout the"),
    ("P", "period. This is an unqualified opinion."),
    ("H3", "Exceptions"),
    ("P", "Our tests identified two exceptions, described in Section 4 for criteria CC6.2"),
    ("P", "and CC7.1. These exceptions did not result in a modification of our opinion."),
    ("P", "Halvorsen & Ruiz LLP, Seattle, Washington, August 14, 2026"),
  ]),
  ("Section 2 Management's Assertion", [
    ("H2", "Section 2 - Management's Assertion"),
    ("P", "We have prepared the accompanying description of Northwind's object storage"),
    ("P", "platform and confirm that the controls were suitably designed and operated"),
    ("P", "effectively throughout the period January 1, 2026 to June 30, 2026."),
    ("P", "Signed: Priya Raman, Chief Executive Officer; Marcus Lee, Chief Security Officer"),
  ]),
  ("Section 3 Description of the System", [
    ("H2", "Section 3 - Description of the System"),
    ("H3", "Services provided"),
    ("P", "Northwind provides encrypted object storage, backup and file sharing APIs to"),
    ("P", "business customers. Customer data is encrypted at rest with AES-256 and in"),
    ("P", "transit with TLS 1.2 or higher."),
    ("H3", "Subservice organizations"),
    ("P", "Subservice organization: Amazon Web Services (infrastructure hosting, carve-out method)"),
    ("P", "Subservice organization: Okta, Inc. (workforce identity, carve-out method)"),
    ("H3", "Complementary user entity controls"),
    ("P", "CUEC-1: Customers are responsible for managing their users' API keys and rotating them."),
    ("P", "CUEC-2: Customers are responsible for removing access for their departed personnel."),
    ("P", "CUEC-3: Customers are responsible for classifying data before uploading it."),
  ]),
  ("Section 4 Trust Services Criteria, Controls and Results", [
    ("H2", "Section 4 - Trust Services Criteria, Controls, Tests and Results"),
    ("P", "Criteria | Control | Result"),
    ("P", "CC1.1 | Code of conduct acknowledged by personnel annually | No exceptions noted"),
    ("P", "CC1.4 | Background checks performed for new hires | No exceptions noted"),
    ("P", "CC2.1 | Security policies reviewed and approved annually | No exceptions noted"),
    ("P", "CC3.1 | Annual risk assessment performed and reviewed by leadership | No exceptions noted"),
    ("P", "CC5.2 | Security awareness training completed on hire and annually | No exceptions noted"),
    ("P", "CC6.1 | MFA enforced for workforce and production access | No exceptions noted"),
    ("P", "CC6.2 | Access removed within one business day of termination | Exception noted"),
    ("P", "CC6.3 | Quarterly user access reviews performed | No exceptions noted"),
  ]),
  ("Section 4 Trust Services Criteria, Controls and Results", [
    ("P", "Criteria | Control | Result"),
    ("P", "CC6.6 | Production network restricted by firewalls and security groups | No exceptions noted"),
    ("P", "CC6.7 | Data encrypted in transit with TLS 1.2 or higher | No exceptions noted"),
    ("P", "CC6.8 | Endpoint protection deployed on workstations | No exceptions noted"),
    ("P", "CC7.1 | Vulnerability scans run monthly and critical findings fixed in 30 days | Exception noted"),
    ("P", "CC7.2 | Security events monitored and alerts triaged | No exceptions noted"),
    ("P", "CC7.4 | Incident response plan tested annually | No exceptions noted"),
    ("P", "CC8.1 | Changes peer reviewed and tested before deployment | No exceptions noted"),
    ("P", "CC9.2 | Vendors assessed before onboarding and annually | No exceptions noted"),
    ("P", "A1.2 | Backups performed daily and restores tested quarterly | No exceptions noted"),
    ("P", "C1.1 | Confidential data identified and retained per schedule | No exceptions noted"),
  ]),
  ("Section 4 Trust Services Criteria, Controls and Results", [
    ("H3", "Description of exceptions"),
    ("P", "Exception CC6.2: For 2 of 25 sampled terminated employees, access to the internal"),
    ("P", "administration console was removed 4 and 6 business days after termination."),
    ("P", "Exception CC7.1: For 1 of 6 monthly scans sampled, two critical vulnerabilities were"),
    ("P", "remediated 41 days after detection, exceeding the 30-day remediation target."),
  ]),
  ("Section 5 Other Information Provided by Management", [
    ("H2", "Section 5 - Other Information Provided by Management"),
    ("H3", "Management's response to exceptions"),
    ("P", "Response CC6.2: Offboarding is now automated from the HR system; console access is"),
    ("P", "revoked through SSO deprovisioning on the termination date."),
    ("P", "Response CC7.1: A dedicated on-call rotation now owns critical vulnerability fixes and"),
    ("P", "progress is reviewed weekly by the Chief Security Officer."),
  ]),
]

def esc(s):
    return s.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")

def page_stream(n, header, lines):
    out = ["BT", "/F1 8 Tf", "50 810 Td", f"({esc('Northwind Cloud Storage - SOC 2 Type II - ' + header)}) Tj", "ET"]
    y = 770
    for kind, text in lines:
        size = {"H1": 22, "H2": 15, "H3": 12, "P": 10}[kind]
        font = "/F2" if kind != "P" else "/F1"
        y -= size + (10 if kind in ("H1", "H2") else 5)
        out += ["BT", f"{font} {size} Tf", f"50 {y} Td", f"({esc(text)}) Tj", "ET"]
    out += ["BT", "/F1 8 Tf", "290 30 Td", f"(Page {n}) Tj", "ET"]
    return "\n".join(out).encode("latin-1")

def build(path):
    objs = []
    def add(b):
        objs.append(b)
        return len(objs)
    font1 = add(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
    font2 = add(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
    pages_id = len(objs) + 1 + 2 * len(PAGES)  # reserved after page objects
    page_ids = []
    for i, (header, lines) in enumerate(PAGES, start=1):
        data = page_stream(i, header, lines)
        content = add(b"<< /Length %d >>\nstream\n" % len(data) + data + b"\nendstream")
        page_ids.append(add(("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> >> >>" % (pages_id, content, font1, font2)).encode()))
    kids = " ".join(f"{p} 0 R" for p in page_ids)
    assert add(f"<< /Type /Pages /Kids [{kids}] /Count {len(page_ids)} >>".encode()) == pages_id
    catalog = add(f"<< /Type /Catalog /Pages {pages_id} 0 R >>".encode())
    info = add(b"<< /Title (Northwind Cloud Storage SOC 2 Type II Report) /Producer (Trustline testdata) >>")
    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = []
    for i, body in enumerate(objs, start=1):
        offsets.append(len(out))
        out += f"{i} 0 obj\n".encode() + body + b"\nendobj\n"
    xref = len(out)
    out += f"xref\n0 {len(objs)+1}\n0000000000 65535 f \n".encode()
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += f"trailer\n<< /Size {len(objs)+1} /Root {catalog} 0 R /Info {info} 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode()
    open(path, "wb").write(out)

if __name__ == "__main__":
    build(sys.argv[1] if len(sys.argv) > 1 else "testdata/vendors/northwind-soc2-type2.pdf")
