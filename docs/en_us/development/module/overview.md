# Overview of BFE Module

## Introduction

- BFE supports plugin architecture that make it possible to develop new features rapidly by writing plugins (i.e. modules).

## How BFE Module works

- Multiple callback points are provided in the forwarding process in BFE.
- When initializing a module, callback functions are registered on specified callback points.
- On processing each request/connection, when reaching a certain callback point, all registered callback functions are executed sequentially.

## Dive into BFE Module

- [BFE callback mechanism](bfe_callback.md)
- [How to write a BFE module](how_to_write_module.md)
