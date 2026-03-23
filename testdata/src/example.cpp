#include <string>

// Shape is a base class for geometric shapes.
class Shape {
public:
    virtual double area() const = 0;
    virtual ~Shape() = default;
};

// Circle represents a circle with a given radius.
class Circle : public Shape {
private:
    double radius;

public:
    Circle(double r) : radius(r) {}
    double area() const override { return 3.14159 * radius * radius; }
};

// computeArea returns the area of a shape.
double computeArea(const Shape& shape) {
    return shape.area();
}

struct Point {
    double x;
    double y;
};

enum Color { Red, Green, Blue };

typedef unsigned int uint;
