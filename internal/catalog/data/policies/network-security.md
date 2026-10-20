---
id: network-security
title: Network Security Policy
category: Operations
summary: Defines how the organization designs, restricts, and protects its networks and cloud network configurations.
controls: [NET-01, NET-02, NET-03, NET-04, CRY-02]
review_cycle_days: 365
---
# Network Security Policy

**Effective date:** {{effective_date}}
**Policy owner:** {{security_officer}}

## Purpose

This policy sets the requirements that limit who and what can reach {{org_name}}'s systems over the network. Its goal is to reduce the attack surface, contain the impact of a compromise, and protect data as it moves between systems.

## Scope

This policy applies to all cloud networks (VPCs, virtual networks, subnets, security groups, firewalls, load balancers), office networks, VPNs, and the personnel who design, configure, or administer them.

## Roles and responsibilities

- **Security Officer ({{security_officer}})** owns this policy, approves network architecture and exceptions, and reviews network configurations.
- **Infrastructure / Operations** implements and maintains network controls, preferably as code.
- **Engineers** must request network changes through the change management process and must not create ad-hoc rules that bypass this policy.

## Policy statements

### Network architecture and segmentation

1. Production, staging, and development environments must be separated into distinct networks or cloud accounts, with no direct network path from non-production to production unless explicitly approved.
2. Within production, systems must be segmented by function into tiers. At a minimum, public-facing components (load balancers, gateways) must be separated from application workloads, and application workloads from data stores.
3. Traffic between segments must be restricted to the specific ports and sources required.
4. Systems processing Restricted data, such as a cardholder data environment, must be placed in a dedicated segment with documented and tested boundaries.
5. A current network diagram showing segments, trust boundaries, and data flows must be maintained and reviewed at least annually.

### Inbound access

1. Inbound access from the internet must be denied by default. Only ports and protocols required for public services, normally HTTPS on port 443, may be open to the internet, and only on designated public-facing components.
2. Administrative ports, including SSH (22), RDP (3389), and database ports, must not be open to 0.0.0.0/0 or ::/0.
3. Administrative access must go through a bastion host, VPN, zero-trust access proxy, or cloud-native session manager that requires multi-factor authentication.
4. Every firewall or security group rule must have a documented purpose. Rules that are not in use must be removed.
5. Firewall and security group rules must be reviewed every {{access_review_days}} days.

### Databases and data stores

1. Databases, caches, and search clusters must not be publicly accessible. They must be deployed in private subnets without public IP addresses.
2. Access to data stores must be limited to the application workloads and administrative paths that require it.
3. Automated checks must detect any publicly accessible database, and any finding must be remediated within 24 hours.

### Outbound traffic

1. Outbound traffic from production should be restricted to known destinations where feasible, and must be logged.

### Instance metadata protection

1. Cloud instances must require session-oriented instance metadata access (for example IMDSv2 on AWS) and disable the legacy metadata service.
2. The metadata hop limit should be set to the minimum required to prevent containers or forwarded requests from reaching credentials.
3. Instance roles must follow least privilege so that a metadata compromise exposes minimal permissions.

### Encryption in transit

1. All external traffic must be encrypted using TLS 1.2 or higher, as defined in the Encryption & Key Management Policy.
2. Internal service-to-service and service-to-database traffic must be encrypted wherever supported.
3. Site-to-site and remote access VPNs must use strong, currently recommended cipher suites.

### Perimeter protection and monitoring

1. Public web applications should be protected by a web application firewall and DDoS protection.
2. Network flow logs must be enabled for production networks and retained in line with the Logging & Monitoring Policy.
3. Network configuration changes must be made through infrastructure-as-code or the change management procedure and must be logged.

### Office and wireless networks

1. Office wireless networks must use WPA2-Enterprise, WPA3, or equivalent, and guest networks must be isolated from corporate resources.
2. Office networks must not be trusted for access to production systems. Production access requires the same authentication as remote access.

## Exceptions

Any deviation, such as a temporary inbound rule for a vendor, must be approved by {{security_officer}}, restricted to specific source addresses, and expire within 30 days unless renewed. Exceptions must be recorded and reviewed.

## Enforcement

Non-compliant network rules may be removed without notice. Personnel who knowingly bypass network controls may be subject to disciplinary action up to and including termination.

## Review

{{security_officer}} reviews this policy at least annually and after significant changes to the network architecture or a network-related security incident.
