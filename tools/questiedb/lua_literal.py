"""Parser for QuestieDB's generated, literal Lua data payloads.

This module never evaluates Lua. Unsupported syntax fails with a location so an
upstream format change cannot silently yield a partial snapshot.
"""

from __future__ import annotations

import re
from dataclasses import dataclass


class LuaLiteralError(ValueError):
    pass


@dataclass(frozen=True)
class Token:
    kind: str
    value: object
    offset: int


_NUMBER = re.compile(r"-?(?:0[xX][0-9a-fA-F]+|(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)")
_IDENT = re.compile(r"[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*")


def _decode_string(raw: str) -> str:
    if raw.startswith("[["):
        return raw[2:-2]
    quote = raw[0]
    chars: list[str] = []
    i = 1
    while i < len(raw) - 1:
        char = raw[i]
        if char != "\\":
            chars.append(char)
            i += 1
            continue
        i += 1
        if i >= len(raw) - 1:
            raise LuaLiteralError("unfinished string escape")
        escaped = raw[i]
        replacements = {"a": "\a", "b": "\b", "f": "\f", "n": "\n", "r": "\r", "t": "\t", "v": "\v", "\\": "\\", "\"": "\"", "'": "'"}
        if escaped in replacements:
            chars.append(replacements[escaped])
            i += 1
        elif escaped == "z":
            i += 1
            while i < len(raw) - 1 and raw[i].isspace():
                i += 1
        elif escaped == "x" and i + 2 < len(raw) - 1:
            try:
                chars.append(chr(int(raw[i + 1 : i + 3], 16)))
            except ValueError as exc:
                raise LuaLiteralError("invalid hexadecimal string escape") from exc
            i += 3
        elif escaped.isdigit():
            digits = raw[i : i + 3]
            match = re.match(r"\d{1,3}", digits)
            assert match is not None
            value = int(match.group(0))
            if value > 255:
                raise LuaLiteralError("decimal string escape exceeds byte range")
            chars.append(chr(value))
            i += len(match.group(0))
        elif escaped == "\n":
            chars.append("\n")
            i += 1
        else:
            raise LuaLiteralError(f"unsupported string escape \\{escaped}")
    return "".join(chars)


def tokenize(text: str, *, preserve_expressions: bool = False) -> list[Token]:
    tokens: list[Token] = []
    i = 0
    while i < len(text):
        char = text[i]
        if char.isspace():
            i += 1
            continue
        if text.startswith("--[[", i):
            end = text.find("]]", i + 4)
            if end < 0:
                raise LuaLiteralError(f"unterminated block comment at byte {i}")
            i = end + 2
            continue
        if text.startswith("--", i):
            end = text.find("\n", i + 2)
            i = len(text) if end < 0 else end + 1
            continue
        if text.startswith("[[", i):
            end = text.find("]]", i + 2)
            if end < 0:
                raise LuaLiteralError(f"unterminated long string at byte {i}")
            raw = text[i : end + 2]
            tokens.append(Token("value", _decode_string(raw), i))
            i = end + 2
            continue
        if char in "\"'":
            start = i
            quote = char
            i += 1
            while i < len(text):
                if text[i] == "\\":
                    i += 2
                elif text[i] == quote:
                    i += 1
                    break
                else:
                    i += 1
            else:
                raise LuaLiteralError(f"unterminated string at byte {start}")
            tokens.append(Token("value", _decode_string(text[start:i]), start))
            continue
        number = _NUMBER.match(text, i)
        if number:
            raw = number.group(0)
            value: object = int(raw, 0) if raw.lower().startswith("0x") else (float(raw) if any(c in raw for c in ".eE") else int(raw))
            tokens.append(Token("value", value, i))
            i = number.end()
            continue
        identifier = _IDENT.match(text, i)
        if identifier:
            tokens.append(Token("identifier", identifier.group(0), i))
            i = identifier.end()
            continue
        if char in "{}[]=,;()" or (preserve_expressions and char in "+-"):
            tokens.append(Token(char, char, i))
            i += 1
            continue
        raise LuaLiteralError(f"unsupported token {char!r} at byte {i}")
    tokens.append(Token("eof", None, len(text)))
    return tokens


class Parser:
    def __init__(self, text: str, *, preserve_expressions: bool = False):
        self.tokens = tokenize(text, preserve_expressions=preserve_expressions)
        self.pos = 0

    @property
    def current(self) -> Token:
        return self.tokens[self.pos]

    def take(self, kind: str) -> Token:
        token = self.current
        if token.kind != kind:
            raise LuaLiteralError(f"expected {kind!r} at byte {token.offset}, got {token.kind!r}")
        self.pos += 1
        return token

    def parse(self) -> object:
        if self.current.kind == "identifier" and self.current.value == "return":
            self.pos += 1
        value = self.value()
        if self.current.kind != "eof":
            raise LuaLiteralError(f"unexpected token at byte {self.current.offset}")
        return value

    def value(self) -> object:
        value = self.primary()
        while self.current.kind == "+":
            self.pos += 1
            value = {"$luaExpression": "+", "arguments": [value, self.primary()]}
        return value

    def primary(self) -> object:
        token = self.current
        if token.kind == "-":
            self.pos += 1
            return {"$luaExpression": "-", "arguments": [self.primary()]}
        if token.kind == "value":
            self.pos += 1
            return token.value
        if token.kind == "{":
            return self.table()
        if token.kind == "identifier":
            name = str(token.value)
            self.pos += 1
            if name == "nil":
                return None
            if name == "true":
                return True
            if name == "false":
                return False
            if self.current.kind == "(":
                self.pos += 1
                arguments = []
                while self.current.kind != ")":
                    arguments.append(self.value())
                    if self.current.kind != ",":
                        break
                    self.pos += 1
                self.take(")")
                return {"$luaExpression": name, "arguments": arguments}
            return {"$luaExpression": name}
        raise LuaLiteralError(f"expected literal value at byte {token.offset}")

    def table(self) -> object:
        self.take("{")
        items: list[object] = []
        keyed: dict[object, object] = {}
        has_keyed = False
        while self.current.kind != "}":
            if self.current.kind == "eof":
                raise LuaLiteralError("unterminated table")
            if self.current.kind == "[":
                self.pos += 1
                key = self.value()
                self.take("]")
                self.take("=")
                keyed[_key(key)] = self.value()
                has_keyed = True
            elif self.current.kind == "identifier" and self.tokens[self.pos + 1].kind == "=":
                key = str(self.current.value)
                self.pos += 2
                keyed[key] = self.value()
                has_keyed = True
            else:
                items.append(self.value())
            if self.current.kind in {",", ";"}:
                self.pos += 1
            elif self.current.kind != "}":
                raise LuaLiteralError(f"expected comma or table end at byte {self.current.offset}")
        self.take("}")
        if not has_keyed:
            return items
        for index, item in enumerate(items, 1):
            keyed[index] = item
        return keyed


def _key(value: object) -> object:
    if isinstance(value, dict) and "$luaExpression" in value and not value.get("arguments"):
        return "$ref:" + str(value["$luaExpression"])
    if isinstance(value, (str, int, float)):
        return value
    raise LuaLiteralError(f"unsupported computed table key {value!r}")


def parse(text: str, *, preserve_expressions: bool = False) -> object:
    return Parser(text, preserve_expressions=preserve_expressions).parse()
