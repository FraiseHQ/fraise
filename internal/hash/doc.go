// MIT License

// Copyright (c) 2026 René-Jean Corneille

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Package hash is the one hashing contract in fraise. Every plan-cache key and
// every graph node key comes from it.
//
// A [Hasher] maps a value to a comparable key under a fixed algorithm and seed.
// [XxHash] (XXH64) and [T1haHash] (t1ha1) implement it, and [NewHasher] picks
// one from the db.hashing-function configuration. A [Hashable] type builds its
// own key material and hashes it once through the Hasher it is given. So a
// composite passes that hasher down to its parts rather than invent a second
// serialisation beside it.
//
// The engine's plan cache depends on that material never colliding. It looks up
// an optimised query by its Hash. Two queries that differ in anything that
// changes the result must yield different material, or a lookup returns another
// query's plan. Graph nodes carry the same burden, since they are stored under
// their Hash. A fact and a topic with the same text stay distinct only because
// each tags its material with its type.
package hash
