# MIT License

# Copyright (c) 2026 René-Jean Corneille

# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

"""Integrations that expose Fraise memory as tools for agent libraries.

Each library has its own submodule, which needs that library's optional extra.
:mod:`fraise_sdk.integrations.openai_agents` needs ``fraise-sdk[openai]``, and
:mod:`fraise_sdk.integrations.claude_agents` needs ``fraise-sdk[anthropic]``.
The top-level package does not import them, so ``import fraise_sdk`` stays free
of those heavy, optional dependencies.

:mod:`fraise_sdk.integrations.render` is the line every recall tool prints for
a fact. It needs no extra, so a tool written for another library can print the
same line.
"""
