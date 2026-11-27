#!/usr/bin/env python3
"""Consume a fill handle without printing the value.

Prints only the length of the value received through AGENT_PWD_FILL and stdin.
"""
import os
import sys

env_value = os.environ.get("AGENT_PWD_FILL", "")
stdin_value = sys.stdin.readline().rstrip("\r\n")
print("env length:", len(env_value))
print("stdin length:", len(stdin_value))
print("same value:", env_value == stdin_value)
