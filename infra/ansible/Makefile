ANSIBLE_CONFIG := $(CURDIR)/ansible.cfg
export ANSIBLE_CONFIG

.PHONY: collections syntax lint test bootstrap bootstrap-root

collections:
	ansible-galaxy collection install -r requirements.yml

syntax:
	ansible-playbook --syntax-check -i inventory/production.yml playbooks/bootstrap.yml

lint:
	ansible-lint .

test:
	python3 -m unittest tests/bootstrap_contract_test.py

bootstrap:
	@test -n "$(PUBLIC_KEY_FILE)" || (echo "PUBLIC_KEY_FILE is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/bootstrap.yml --extra-vars "bootstrap_public_key=$$(cat '$(PUBLIC_KEY_FILE)')"

# First run only: connect as root with an interactive SSH password prompt.
bootstrap-root:
	@test -n "$(PUBLIC_KEY_FILE)" || (echo "PUBLIC_KEY_FILE is required" >&2; exit 2)
	ansible-playbook -i inventory/production.yml playbooks/bootstrap.yml --user root --ask-pass --extra-vars "bootstrap_public_key=$$(cat '$(PUBLIC_KEY_FILE)')"
